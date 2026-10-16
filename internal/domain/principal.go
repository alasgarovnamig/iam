package domain

// PrincipalKind mirrors the token principal_type.
type PrincipalKind string

const (
	KindPlatformUser   PrincipalKind = "platform_user"
	KindTenantUser     PrincipalKind = "tenant_user"
	KindAPIClient      PrincipalKind = "api_client"
	KindServiceAccount PrincipalKind = "service_account"
	KindSystemService  PrincipalKind = "system_service"
)

// Principal is the authenticated caller identity attached to each request.
type Principal struct {
	ID        string
	Kind      PrincipalKind
	TenantID  string // empty for platform principals
	SessionID string
	ClientID  string
	Scopes    []string
	Email     string
}

func (p Principal) IsPlatform() bool {
	return p.Kind == KindPlatformUser || p.Kind == KindSystemService
}

func (p Principal) IsAPIClient() bool { return p.Kind == KindAPIClient }
