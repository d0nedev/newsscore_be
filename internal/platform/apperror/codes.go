package apperror

// Error codes shared by every domain. They are part of the API contract
// (see openapi.yaml): changing a value is a breaking change for clients.
// Domain-specific codes live in their own package, e.g. <domain>/errors.go.
const (
	CodeValidation          = "VALIDATION_ERROR"
	CodeInternal            = "INTERNAL_SERVER_ERROR"
	CodeNotFound            = "NOT_FOUND"
	CodeMethodNotAllowed    = "METHOD_NOT_ALLOWED"
	CodeUnauthorized        = "UNAUTHORIZED"
	CodeForbidden           = "FORBIDDEN"
	CodeRateLimited         = "RATE_LIMITED"
	CodeClientIPUnresolved  = "CLIENT_IP_UNRESOLVED"
	CodeRequestBodyTooLarge = "REQUEST_BODY_TOO_LARGE"
)
