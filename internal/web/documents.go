package web

import (
	"errors"
	"net/http"
	"os"
	"strconv"

	"kb-rag/internal/ingest"
	"kb-rag/internal/ingest/loader"
	"kb-rag/internal/store"
)

func (s *Server) handleUploadDocument(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, s.maxUploadBytes+1<<20)
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			writeError(w, http.StatusRequestEntityTooLarge, "文件超过大小限制")
			return
		}
		writeError(w, http.StatusBadRequest, "上传表单格式错误")
		return
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "缺少文件字段 file")
		return
	}
	defer file.Close()

	if !loader.Supported(header.Filename) {
		writeError(w, http.StatusUnsupportedMediaType, "不支持的文件类型")
		return
	}
	if header.Size <= 0 {
		writeError(w, http.StatusBadRequest, "文件为空")
		return
	}
	if header.Size > s.maxUploadBytes {
		writeError(w, http.StatusRequestEntityTooLarge, "文件超过大小限制")
		return
	}

	target, err := s.saveUpload(file, header.Filename)
	if err != nil {
		s.logger.Error("保存上传文件失败", "error", err)
		writeError(w, http.StatusInternalServerError, "保存上传文件失败")
		return
	}

	s.ingestMu.Lock()
	res := ingest.IngestFile(r.Context(), target, s.store, s.embedder, s.ingestOptions)
	s.ingestMu.Unlock()
	if res.Fatal {
		s.logger.Error("上传入库失败", "error", res.Err)
		writeError(w, http.StatusInternalServerError, "入库失败")
		return
	}

	info, ok, err := s.store.GetDocument(target)
	if err != nil || !ok {
		s.logger.Error("读取上传文档失败", "error", err, "path", target)
		writeError(w, http.StatusInternalServerError, "读取上传文档失败")
		return
	}
	summary, ok, err := s.summaryByID(r.Context(), info.ID)
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

func parseDocumentID(r *http.Request) (int64, error) {
	return strconv.ParseInt(r.PathValue("id"), 10, 64)
}

func (s *Server) handleDeleteDocument(w http.ResponseWriter, r *http.Request) {
	id, err := parseDocumentID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "文档 ID 非法")
		return
	}
	if err := s.store.DeleteDocument(r.Context(), id); err != nil {
		if errors.Is(err, store.ErrDocumentNotFound) {
			writeError(w, http.StatusNotFound, "文档不存在")
			return
		}
		s.logger.Error("删除文档失败", "error", err, "id", id)
		writeError(w, http.StatusInternalServerError, "删除文档失败")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleReingestDocument(w http.ResponseWriter, r *http.Request) {
	id, err := parseDocumentID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "文档 ID 非法")
		return
	}
	info, ok, err := s.store.GetDocumentByID(r.Context(), id)
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

	s.ingestMu.Lock()
	res := ingest.IngestFile(r.Context(), info.Path, s.store, s.embedder, s.ingestOptions)
	s.ingestMu.Unlock()
	if res.Fatal {
		s.logger.Error("重试入库失败", "error", res.Err, "id", id)
		writeError(w, http.StatusInternalServerError, "重试入库失败")
		return
	}

	summary, ok, err := s.summaryByID(r.Context(), id)
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
