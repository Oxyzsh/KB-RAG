package web

type embeddingDTO struct {
	Model     string `json:"model"`
	Dim       int    `json:"dim"`
	ChunkSize int    `json:"chunk_size"`
}

type statsDTO struct {
	Documents       int          `json:"documents"`
	DocumentsFailed int          `json:"documents_failed"`
	Chunks          int          `json:"chunks"`
	SizeBytes       int64        `json:"size_bytes"`
	TenantSizeBytes int64        `json:"tenant_size_bytes"`
	Embedding       embeddingDTO `json:"embedding"`
}

type documentDTO struct {
	ID        int64  `json:"id"`
	Title     string `json:"title"`
	Name      string `json:"name"`
	Status    string `json:"status"`
	Chunks    int    `json:"chunks"`
	Source    string `json:"source"`
	Error     string `json:"error"`
	UpdatedAt string `json:"updated_at"`
}

type listDocumentsResponse struct {
	Documents []documentDTO `json:"documents"`
}

type citationDTO struct {
	N       int     `json:"n"`
	Source  string  `json:"source"`
	Heading string  `json:"heading"`
	Text    string  `json:"text"`
	Score   float64 `json:"score"`
}

type askRequest struct {
	Question string `json:"question"`
	TopK     int    `json:"top_k"`
}

type askResponse struct {
	Answer    string        `json:"answer"`
	Generated bool          `json:"generated"`
	Warning   string        `json:"warning"`
	Citations []citationDTO `json:"citations"`
}

type uploadResponse struct {
	Document documentDTO `json:"document"`
	Outcome  string      `json:"outcome"`
}

type errorResponse struct {
	Error string `json:"error"`
}

type tenantDTO struct {
	ID   int64  `json:"id"`
	Slug string `json:"slug"`
	Name string `json:"name"`
}

type userDTO struct {
	ID             int64  `json:"id"`
	Username       string `json:"username"`
	Role           string `json:"role"`
	IsSuper        bool   `json:"is_super"`
	TenantID       int64  `json:"tenant_id"`
	TenantSlug     string `json:"tenant_slug"`
	TenantName     string `json:"tenant_name"`
	HomeTenantID   int64  `json:"home_tenant_id"`
	HomeTenantSlug string `json:"home_tenant_slug"`
	HomeTenantName string `json:"home_tenant_name"`
	CanSwitch      bool   `json:"can_switch"`
	CreatedAt      string `json:"created_at"`
	UpdatedAt      string `json:"updated_at"`
}

type loginRequest struct {
	Tenant   string `json:"tenant"`
	Username string `json:"username"`
	Password string `json:"password"`
}

type loginResponse struct {
	Token     string  `json:"token"`
	ExpiresAt string  `json:"expires_at"`
	User      userDTO `json:"user"`
}

type sessionResponse struct {
	User userDTO `json:"user"`
}

type userListResponse struct {
	Users []userDTO `json:"users"`
}

type userResponse struct {
	User userDTO `json:"user"`
}

type createUserRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Role     string `json:"role"`
}

type updateUserRequest struct {
	Role     *string `json:"role"`
	Password *string `json:"password"`
}

type tenantsResponse struct {
	Tenants []tenantDTO `json:"tenants"`
}

type createTenantRequest struct {
	Slug          string `json:"slug"`
	Name          string `json:"name"`
	AdminUsername string `json:"admin_username"`
	AdminPassword string `json:"admin_password"`
}

type createTenantResponse struct {
	Tenant tenantDTO `json:"tenant"`
	User   userDTO   `json:"user"`
}

type resetTenantAdminRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type auditDTO struct {
	ID         int64  `json:"id"`
	TenantID   *int64 `json:"tenant_id"`
	TenantSlug string `json:"tenant_slug"`
	UserID     *int64 `json:"user_id"`
	Username   string `json:"username"`
	Action     string `json:"action"`
	Target     string `json:"target"`
	Detail     string `json:"detail"`
	IP         string `json:"ip"`
	CreatedAt  string `json:"created_at"`
}

type auditListResponse struct {
	Items    []auditDTO `json:"items"`
	Total    int        `json:"total"`
	Page     int        `json:"page"`
	PageSize int        `json:"page_size"`
}
