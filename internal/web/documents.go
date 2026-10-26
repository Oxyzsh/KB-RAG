package web

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"kb-rag/internal/ingest"
	"kb-rag/internal/ingest/loader"
	"kb-rag/internal/store"
)

func (s *Server) receiveUpload(w http.ResponseWriter, r *http.Request) (string, string, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, s.maxUploadBytes+1<<20)
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			writeError(w, http.StatusRequestEntityTooLarge, "文件超过大小限制")
			return "", "", false
		}
		writeError(w, http.StatusBadRequest, "上传表单格式错误")
		return "", "", false
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "缺少文件字段 file")
		return "", "", false
	}
	defer file.Close()

	if !loader.Supported(header.Filename) {
		writeError(w, http.StatusUnsupportedMediaType, "不支持的文件类型")
		return "", "", false
	}
	if header.Size <= 0 {
		writeError(w, http.StatusBadRequest, "文件为空")
		return "", "", false
	}
	if header.Size > s.maxUploadBytes {
		writeError(w, http.StatusRequestEntityTooLarge, "文件超过大小限制")
		return "", "", false
	}

	target, err := s.saveUpload(file, header.Filename)
	if err != nil {
		s.logger.Error("保存上传文件失败", "error", err)
		writeError(w, http.StatusInternalServerError, "保存上传文件失败")
		return "", "", false
	}
	return target, filepath.Base(target), true
}

func (s *Server) handleUploadDocument(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r.Context())
	target, name, ok := s.receiveUpload(w, r)
	if !ok {
		return
	}

	opts := s.ingestOptions
	opts.TenantID = p.TenantID
	s.ingestMu.Lock()
	res := ingest.IngestFile(r.Context(), target, s.store, s.embedder, opts)
	s.ingestMu.Unlock()
	s.audit(r, p, p.TenantID, "document.upload", name, map[string]any{
		"outcome": res.Outcome,
		"chunks":  res.Chunks,
	})
	if res.Fatal {
		s.logger.Error("上传入库失败", "error", res.Err)
		writeError(w, http.StatusInternalServerError, "入库失败")
		return
	}

	info, ok, err := s.store.GetDocument(r.Context(), p.TenantID, target)
	if err != nil || !ok {
		s.logger.Error("读取上传文档失败", "error", err)
		writeError(w, http.StatusInternalServerError, "读取上传文档失败")
		return
	}
	summary, ok, err := s.store.GetDocumentSummaryByID(r.Context(), p.TenantID, info.ID)
	if err != nil || !ok {
		s.logger.Error("读取上传文档摘要失败", "error", err, "id", info.ID)
		writeError(w, http.StatusInternalServerError, "读取上传文档失败")
		return
	}

	writeJSON(w, http.StatusCreated, uploadResponse{
		Document: s.documentDTO(summary),
		Outcome:  res.Outcome,
	})
}

func (s *Server) handleUploadJob(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r.Context())
	target, name, ok := s.receiveUpload(w, r)
	if !ok {
		return
	}
	job, err := s.store.CreateJob(r.Context(), store.Job{
		TenantID: p.TenantID,
		UserID:   p.UserID,
		Username: p.Username,
		Action:   "document.upload",
		Path:     target,
		Name:     name,
		IP:       clientIP(r),
	})
	if err != nil {
		s.logger.Error("创建上传任务失败", "error", err)
		writeError(w, http.StatusInternalServerError, "创建上传任务失败")
		return
	}
	writeJSON(w, http.StatusAccepted, jobResponse{Job: jobDTOFromStore(job)})
}

func parseDocumentID(r *http.Request) (int64, error) {
	return strconv.ParseInt(r.PathValue("id"), 10, 64)
}

func (s *Server) handleDeleteDocument(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r.Context())
	id, err := parseDocumentID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "文档 ID 非法")
		return
	}
	summary, ok, err := s.store.GetDocumentSummaryByID(r.Context(), p.TenantID, id)
	if err != nil {
		s.logger.Error("读取文档失败", "error", err, "id", id)
		writeError(w, http.StatusInternalServerError, "读取文档失败")
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "文档不存在")
		return
	}
	if err := s.store.DeleteDocument(r.Context(), p.TenantID, id); err != nil {
		if errors.Is(err, store.ErrDocumentNotFound) {
			writeError(w, http.StatusNotFound, "文档不存在")
			return
		}
		s.logger.Error("删除文档失败", "error", err, "id", id)
		writeError(w, http.StatusInternalServerError, "删除文档失败")
		return
	}
	s.audit(r, p, p.TenantID, "document.delete", filepath.Base(summary.Path), map[string]any{
		"document_id": id,
		"title":       summary.Title,
	})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleReingestDocument(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r.Context())
	id, err := parseDocumentID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "文档 ID 非法")
		return
	}
	info, ok, err := s.store.GetDocumentByID(r.Context(), p.TenantID, id)
	if err != nil {
		s.logger.Error("读取文档失败", "error", err, "id", id)
		writeError(w, http.StatusInternalServerError, "读取文档失败")
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "文档不存在")
		return
	}
	if _, err := os.Stat(info.Path); err != nil {
		writeError(w, http.StatusConflict, "源文件不存在，无法重试")
		return
	}

	opts := s.ingestOptions
	opts.TenantID = p.TenantID
	s.ingestMu.Lock()
	res := ingest.IngestFile(r.Context(), info.Path, s.store, s.embedder, opts)
	s.ingestMu.Unlock()
	s.audit(r, p, p.TenantID, "document.reingest", filepath.Base(info.Path), map[string]any{
		"document_id": id,
		"outcome":     res.Outcome,
		"chunks":      res.Chunks,
	})
	if res.Fatal {
		s.logger.Error("重试入库失败", "error", res.Err, "id", id)
		writeError(w, http.StatusInternalServerError, "重试入库失败")
		return
	}

	summary, ok, err := s.store.GetDocumentSummaryByID(r.Context(), p.TenantID, id)
	if err != nil || !ok {
		s.logger.Error("读取重试结果失败", "error", err, "id", id)
		writeError(w, http.StatusInternalServerError, "读取重试结果失败")
		return
	}
	writeJSON(w, http.StatusOK, uploadResponse{
		Document: s.documentDTO(summary),
		Outcome:  res.Outcome,
	})
}

func (s *Server) handleReingestJob(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r.Context())
	id, err := parseDocumentID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "文档 ID 非法")
		return
	}
	info, ok, err := s.store.GetDocumentByID(r.Context(), p.TenantID, id)
	if err != nil {
		s.logger.Error("读取文档失败", "error", err, "id", id)
		writeError(w, http.StatusInternalServerError, "读取文档失败")
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "文档不存在")
		return
	}
	if _, err := os.Stat(info.Path); err != nil {
		writeError(w, http.StatusConflict, "源文件不存在，无法重试")
		return
	}
	job, err := s.store.CreateJob(r.Context(), store.Job{
		TenantID: p.TenantID,
		UserID:   p.UserID,
		Username: p.Username,
		Action:   "document.reingest",
		Path:     info.Path,
		Name:     filepath.Base(info.Path),
		IP:       clientIP(r),
	})
	if err != nil {
		s.logger.Error("创建重试任务失败", "error", err)
		writeError(w, http.StatusInternalServerError, "创建重试任务失败")
		return
	}
	writeJSON(w, http.StatusAccepted, jobResponse{Job: jobDTOFromStore(job)})
}

func (s *Server) handleGetJob(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r.Context())
	id, err := parseDocumentID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "任务 ID 非法")
		return
	}
	job, ok, err := s.store.GetJob(r.Context(), p.TenantID, id)
	if err != nil {
		s.logger.Error("读取任务失败", "error", err, "id", id)
		writeError(w, http.StatusInternalServerError, "读取任务失败")
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "任务不存在")
		return
	}
	writeJSON(w, http.StatusOK, jobResponse{Job: jobDTOFromStore(job)})
}

func (s *Server) handleCancelJob(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r.Context())
	id, err := parseDocumentID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "任务 ID 非法")
		return
	}
	job, ok, err := s.store.GetJob(r.Context(), p.TenantID, id)
	if err != nil {
		s.logger.Error("读取任务失败", "error", err, "id", id)
		writeError(w, http.StatusInternalServerError, "读取任务失败")
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "任务不存在")
		return
	}
	if job.Status == store.JobQueued {
		canceled, err := s.store.CancelQueuedJob(r.Context(), p.TenantID, id)
		if err != nil {
			s.logger.Error("取消任务失败", "error", err, "id", id)
			writeError(w, http.StatusInternalServerError, "取消任务失败")
			return
		}
		if canceled {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		job, ok, err = s.store.GetJob(r.Context(), p.TenantID, id)
		if err != nil || !ok {
			writeError(w, http.StatusNotFound, "任务不存在")
			return
		}
	}
	if job.Status == store.JobRunning && s.cancelRunningJob(id) {
		w.WriteHeader(http.StatusAccepted)
		return
	}
	writeError(w, http.StatusConflict, "任务已结束，无法取消")
}
