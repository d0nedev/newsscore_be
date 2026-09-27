package news

// News error codes, part of the API contract (see openapi.yaml).
const (
	CodeNewsNotFound    = "NEWS_NOT_FOUND"
	CodeNewsSlugTaken   = "NEWS_SLUG_TAKEN"
	CodeNewsQueryFailed = "NEWS_QUERY_FAILED"
	CodeNewsWriteFailed = "NEWS_WRITE_FAILED"
)
