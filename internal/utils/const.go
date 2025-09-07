package utils

const (
	QUOTA_STEP int64 = 1
)

const (
	SERVICE_DEFAULT_PAGE_SIZE = 50
	SERVICE_MAX_PAGE_SIZE     = 100

	PAGE_TOKEN_DEFAULT_EXP_SECONDS = 24 * 3600
)

type CtxKey int64

const (
	KEY_ACCOUNT_ID CtxKey = iota
)
