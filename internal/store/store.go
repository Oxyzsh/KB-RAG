package store

import (
	"context"
	"database/sql"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

const SchemaVersion = "3"

type Store struct {
	db   *sql.DB
	path string
}

type EmbeddingMeta struct {
	SchemaVersion string
	Model         string
	Dim           int
	ChunkSize     int
}

type DocumentInfo struct {
	ID        int64
	Path      string
	Title     string
	Hash      string
	Status    string
	Error     string
	UpdatedAt string
}

type ChunkInput struct {
	Seq     int
	Heading string
	Text    string
	Vector  []float32
}

type Hit struct {
	ChunkID int64
	DocID   int64
	Path    string
	Title   string
	Heading string
	Text    string
	Score   float64
}

type Stats struct {
	Documents       int
	DocumentsFailed int
	Chunks          int
	SizeBytes       int64
	TenantSizeBytes int64
	FailedPaths     []string
	Meta            EmbeddingMeta
}

type DocumentSummary struct {
	ID        int64
	Path      string
	Title     string
	Status    string
	Error     string
	UpdatedAt string
	Chunks    int
}

var ErrDocumentNotFound = errors.New("文档不存在")

func Open(path string) (*Store, error) {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("创建库目录 %s: %w", dir, err)
		}
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("打开 SQLite: %w", err)
	}
	db.SetMaxOpenConns(1)
	var mode string
	if err := db.QueryRow("PRAGMA journal_mode=WAL").Scan(&mode); err != nil {
		db.Close()
		return nil, fmt.Errorf("启用 WAL: %w", err)
	}
	for _, pragma := range []string{"PRAGMA busy_timeout=5000", "PRAGMA foreign_keys=ON"} {
		if _, err := db.Exec(pragma); err != nil {
			db.Close()
			return nil, fmt.Errorf("%s: %w", pragma, err)
		}
	}
	s := &Store{db: db, path: path}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) migrate() error {
	hasMeta, err := tableExists(s.db, "meta")
	if err != nil {
		return err
	}
	if !hasMeta {
		hasDocs, err := tableExists(s.db, "documents")
		if err != nil {
			return err
		}
		if hasDocs {
			return fmt.Errorf("数据库缺少 meta 表，无法确认版本；请恢复备份")
		}
		return s.initFreshV3(context.Background())
	}
	version, ok, err := s.metaGet("schema_version")
	if err != nil {
		return err
	}
	if !ok {
		hasDocs, err := tableExists(s.db, "documents")
		if err != nil {
			return err
		}
		if hasDocs {
			return fmt.Errorf("数据库缺少 schema_version，无法确认版本；请恢复备份")
		}
		return s.initFreshV3(context.Background())
	}
	switch version {
	case SchemaVersion:
		return nil
	case "2":
		return s.migrateV2ToV3(context.Background())
	default:
		return fmt.Errorf("数据库 schema 版本 %s 与程序 %s 不一致，请恢复备份或重新 ingest", version, SchemaVersion)
	}
}

func (s *Store) metaGet(key string) (string, bool, error) {
	var value string
	err := s.db.QueryRow(`SELECT value FROM meta WHERE key=?`, key).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return value, true, nil
}

func (s *Store) metaSet(key, value string) error {
	_, err := s.db.Exec(`INSERT INTO meta(key,value) VALUES(?,?)
		ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, value)
	return err
}

func (s *Store) InitMeta(model string, chunkSize int) error {
	if model == "" || chunkSize <= 0 {
		return errors.New("嵌入模型和 chunk_size 必须有效")
	}
	currentModel, ok, err := s.metaGet("embedding_model")
	if err != nil {
		return err
	}
	if ok && currentModel != model {
		return fmt.Errorf("嵌入模型不一致：库内为 %q，配置为 %q；请删除库文件后重新 ingest", currentModel, model)
	}
	if !ok {
		if err := s.metaSet("embedding_model", model); err != nil {
			return err
		}
	}
	currentSize, ok, err := s.metaGet("chunk_size")
	if err != nil {
		return err
	}
	if ok {
		size, err := strconv.Atoi(currentSize)
		if err != nil {
			return fmt.Errorf("库内 chunk_size 值 %q 非法", currentSize)
		}
		if size != chunkSize {
			return fmt.Errorf("切分参数不一致：库内为 %d，配置为 %d；请删除库文件后重新 ingest", size, chunkSize)
		}
	} else if err := s.metaSet("chunk_size", strconv.Itoa(chunkSize)); err != nil {
		return err
	}
	return nil
}

func (s *Store) Meta() (EmbeddingMeta, error) {
	var meta EmbeddingMeta
	rows, err := s.db.Query(`SELECT key,value FROM meta`)
	if err != nil {
		return meta, err
	}
	defer rows.Close()
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return meta, err
		}
		switch key {
		case "schema_version":
			meta.SchemaVersion = value
		case "embedding_model":
			meta.Model = value
		case "dim":
			dim, err := strconv.Atoi(value)
			if err != nil {
				return meta, fmt.Errorf("meta.dim 值 %q 非法", value)
			}
			meta.Dim = dim
		case "chunk_size":
			size, err := strconv.Atoi(value)
			if err != nil {
				return meta, fmt.Errorf("meta.chunk_size 值 %q 非法", value)
			}
			meta.ChunkSize = size
		}
	}
	return meta, rows.Err()
}

func (s *Store) SetDim(dim int) error {
	if dim <= 0 {
		return fmt.Errorf("向量维度必须大于 0")
	}
	current, ok, err := s.metaGet("dim")
	if err != nil {
		return err
	}
	if !ok {
		return s.metaSet("dim", strconv.Itoa(dim))
	}
	n, err := strconv.Atoi(current)
	if err != nil {
		return fmt.Errorf("meta.dim 值 %q 非法", current)
	}
	if n != dim {
		return fmt.Errorf("嵌入维度不一致：库内为 %d，本次为 %d；请删除库文件后重新 ingest", n, dim)
	}
	return nil
}

func (s *Store) GetDocument(ctx context.Context, tenantID int64, path string) (DocumentInfo, bool, error) {
	var info DocumentInfo
	err := s.db.QueryRowContext(ctx, `SELECT id,path,title,content_hash,status,error,updated_at FROM documents WHERE tenant_id=? AND path_key=?`, tenantID, pathKey(path)).
		Scan(&info.ID, &info.Path, &info.Title, &info.Hash, &info.Status, &info.Error, &info.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return DocumentInfo{}, false, nil
	}
	if err != nil {
		return DocumentInfo{}, false, err
	}
	return info, true, nil
}

func (s *Store) ReplaceDocument(ctx context.Context, tenantID int64, path, title, hash string, chunks []ChunkInput) error {
	dim, err := s.dim()
	if err != nil {
		return err
	}
	if dim <= 0 {
		return errors.New("嵌入维度尚未初始化")
	}
	for i, chunk := range chunks {
		if len(chunk.Vector) != dim {
			return fmt.Errorf("第 %d 个片段维度为 %d，期望 %d", i+1, len(chunk.Vector), dim)
		}
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := tx.ExecContext(ctx, `INSERT INTO documents(tenant_id,path,path_key,title,content_hash,status,error,updated_at)
		VALUES(?,?,?,?,?,'ok','',?)
		ON CONFLICT(tenant_id,path_key) DO UPDATE SET path=excluded.path, title=excluded.title, content_hash=excluded.content_hash,
			status='ok', error='', updated_at=excluded.updated_at`, tenantID, path, pathKey(path), title, hash, now); err != nil {
		return err
	}
	var docID int64
	if err := tx.QueryRowContext(ctx, `SELECT id FROM documents WHERE tenant_id=? AND path_key=?`, tenantID, pathKey(path)).Scan(&docID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM chunks WHERE doc_id=?`, docID); err != nil {
		return err
	}
	stmt, err := tx.PrepareContext(ctx, `INSERT INTO chunks(doc_id,seq,heading,text,vector,dim) VALUES(?,?,?,?,?,?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, chunk := range chunks {
		if _, err := stmt.ExecContext(ctx, docID, chunk.Seq, chunk.Heading, chunk.Text, encodeVector(normalize(chunk.Vector)), dim); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) MarkFailed(ctx context.Context, tenantID int64, path, title, hash, msg string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.ExecContext(ctx, `INSERT INTO documents(tenant_id,path,path_key,title,content_hash,status,error,updated_at)
		VALUES(?,?,?,?,?,'failed',?,?)
		ON CONFLICT(tenant_id,path_key) DO UPDATE SET path=excluded.path, title=excluded.title, content_hash=excluded.content_hash,
			status='failed', error=excluded.error, updated_at=excluded.updated_at`, tenantID, path, pathKey(path), title, hash, msg, now)
	return err
}

func (s *Store) Search(ctx context.Context, tenantID int64, query []float32, topK int) ([]Hit, error) {
	if topK <= 0 {
		return nil, fmt.Errorf("top-k 必须大于 0")
	}
	dim, err := s.dim()
	if err != nil {
		return nil, err
	}
	if dim <= 0 {
		return nil, errors.New("知识库尚未完成初始化，请先执行 ingest")
	}
	if len(query) != dim {
		return nil, fmt.Errorf("查询向量维度为 %d，库内为 %d", len(query), dim)
	}
	q := normalize(query)
	rows, err := s.db.QueryContext(ctx, `SELECT c.id,c.doc_id,c.heading,c.text,c.vector,c.dim,d.path,d.title
		FROM chunks c JOIN documents d ON d.id=c.doc_id WHERE d.status='ok' AND d.tenant_id=?`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var hits []Hit
	for rows.Next() {
		var (
			hit      Hit
			vector   []byte
			chunkDim int
		)
		if err := rows.Scan(&hit.ChunkID, &hit.DocID, &hit.Heading, &hit.Text, &vector, &chunkDim, &hit.Path, &hit.Title); err != nil {
			return nil, err
		}
		if chunkDim != dim {
			return nil, fmt.Errorf("片段 %d 维度为 %d，库内为 %d", hit.ChunkID, chunkDim, dim)
		}
		decoded, err := decodeVector(vector)
		if err != nil {
			return nil, err
		}
		if len(decoded) != dim {
			return nil, fmt.Errorf("片段 %d 向量长度为 %d，库内维度为 %d", hit.ChunkID, len(decoded), dim)
		}
		var score float64
		for i := range q {
			score += float64(q[i]) * float64(decoded[i])
		}
		hit.Score = score
		hits = append(hits, hit)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.Slice(hits, func(i, j int) bool { return hits[i].Score > hits[j].Score })
	if len(hits) > topK {
		hits = hits[:topK]
	}
	return hits, nil
}

func (s *Store) Stats(ctx context.Context, tenantID int64) (Stats, error) {
	var stats Stats
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM documents WHERE tenant_id=?`, tenantID).Scan(&stats.Documents); err != nil {
		return stats, err
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM documents WHERE tenant_id=? AND status='failed'`, tenantID).Scan(&stats.DocumentsFailed); err != nil {
		return stats, err
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM chunks c JOIN documents d ON d.id=c.doc_id WHERE d.tenant_id=?`, tenantID).Scan(&stats.Chunks); err != nil {
		return stats, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT path FROM documents WHERE tenant_id=? AND status='failed' ORDER BY path`, tenantID)
	if err != nil {
		return stats, err
	}
	for rows.Next() {
		var path string
		if err := rows.Scan(&path); err != nil {
			rows.Close()
			return stats, err
		}
		stats.FailedPaths = append(stats.FailedPaths, path)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return stats, err
	}
	rows.Close()
	if err := s.db.QueryRowContext(ctx, `SELECT COALESCE(SUM(LENGTH(c.text)+LENGTH(c.heading)+LENGTH(c.vector)+LENGTH(d.path)+LENGTH(d.title)),0)
		FROM chunks c JOIN documents d ON d.id=c.doc_id WHERE d.tenant_id=?`, tenantID).Scan(&stats.TenantSizeBytes); err != nil {
		return stats, err
	}
	for _, suffix := range []string{"", "-wal", "-shm"} {
		if info, err := os.Stat(s.path + suffix); err == nil {
			stats.SizeBytes += info.Size()
		}
	}
	meta, err := s.Meta()
	if err != nil {
		return stats, err
	}
	stats.Meta = meta
	return stats, nil
}

func (s *Store) ListDocuments(ctx context.Context, tenantID int64) ([]DocumentSummary, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT d.id,d.path,d.title,d.status,d.error,d.updated_at,COUNT(c.id)
		FROM documents d LEFT JOIN chunks c ON c.doc_id=d.id
		WHERE d.tenant_id=?
		GROUP BY d.id,d.path,d.title,d.status,d.error,d.updated_at
		ORDER BY d.updated_at DESC, d.id DESC`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []DocumentSummary
	for rows.Next() {
		var d DocumentSummary
		if err := rows.Scan(&d.ID, &d.Path, &d.Title, &d.Status, &d.Error, &d.UpdatedAt, &d.Chunks); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (s *Store) GetDocumentSummaryByID(ctx context.Context, tenantID, id int64) (DocumentSummary, bool, error) {
	var d DocumentSummary
	err := s.db.QueryRowContext(ctx, `SELECT d.id,d.path,d.title,d.status,d.error,d.updated_at,COUNT(c.id)
		FROM documents d LEFT JOIN chunks c ON c.doc_id=d.id
		WHERE d.id=? AND d.tenant_id=?
		GROUP BY d.id,d.path,d.title,d.status,d.error,d.updated_at`, id, tenantID).
		Scan(&d.ID, &d.Path, &d.Title, &d.Status, &d.Error, &d.UpdatedAt, &d.Chunks)
	if errors.Is(err, sql.ErrNoRows) {
		return DocumentSummary{}, false, nil
	}
	if err != nil {
		return DocumentSummary{}, false, err
	}
	return d, true, nil
}

func (s *Store) GetDocumentByID(ctx context.Context, tenantID, id int64) (DocumentInfo, bool, error) {
	var info DocumentInfo
	err := s.db.QueryRowContext(ctx, `SELECT id,path,title,content_hash,status,error,updated_at
		FROM documents WHERE id=? AND tenant_id=?`, id, tenantID).
		Scan(&info.ID, &info.Path, &info.Title, &info.Hash, &info.Status, &info.Error, &info.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return DocumentInfo{}, false, nil
	}
	if err != nil {
		return DocumentInfo{}, false, err
	}
	return info, true, nil
}

func (s *Store) DeleteDocument(ctx context.Context, tenantID, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM documents WHERE id=? AND tenant_id=?`, id, tenantID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrDocumentNotFound
	}
	return nil
}

func (s *Store) dim() (int, error) {
	meta, err := s.Meta()
	if err != nil {
		return 0, err
	}
	return meta.Dim, nil
}

// pathKey 是文件身份：绝对路径在 Windows 上折叠大小写，避免同一文件重复入库。
// 展示路径仍保存在 documents.path 中，保持用户传入的原始大小写。
func pathKey(path string) string {
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	path = filepath.Clean(path)
	if runtime.GOOS == "windows" {
		path = strings.ToLower(path)
	}
	return path
}

func encodeVector(vector []float32) []byte {
	encoded := make([]byte, 4*len(vector))
	for i, value := range vector {
		binary.LittleEndian.PutUint32(encoded[i*4:], math.Float32bits(value))
	}
	return encoded
}

func decodeVector(encoded []byte) ([]float32, error) {
	if len(encoded)%4 != 0 {
		return nil, fmt.Errorf("向量字节长度 %d 不是 4 的倍数", len(encoded))
	}
	vector := make([]float32, len(encoded)/4)
	for i := range vector {
		vector[i] = math.Float32frombits(binary.LittleEndian.Uint32(encoded[i*4:]))
	}
	return vector, nil
}

func normalize(vector []float32) []float32 {
	out := make([]float32, len(vector))
	var sum float64
	for _, value := range vector {
		sum += float64(value) * float64(value)
	}
	if sum == 0 {
		copy(out, vector)
		return out
	}
	scale := float32(1 / math.Sqrt(sum))
	for i, value := range vector {
		out[i] = value * scale
	}
	return out
}
