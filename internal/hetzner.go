package internal

import "strconv"

const (
	DefaultApiUrl    = "https://api.hetzner.cloud/v1"
	DefaultSecretKey = "api-token"
	DefaultTxtTTL    = 120
)

type Config struct {
	ApiKey    string
	ZoneName  string
	ZoneId    int64
	ApiUrl    string
	SecretKey string
}

func (c *Config) ZoneIdStr() string {
	return strconv.FormatInt(c.ZoneId, 10)
}

type ZoneListResponse struct {
	Zones []Zone `json:"zones"`
}

type ZoneResponse struct {
	Zone Zone `json:"zone"`
}

type Zone struct {
	Id          int64  `json:"id"`
	Name        string `json:"name"`
	TTL         int    `json:"ttl"`
	RecordCount int    `json:"record_count"`
}

type RRSetListResponse struct {
	RRSets []RRSet `json:"rrsets"`
}

type RRSetResponse struct {
	RRSet RRSet `json:"rrset"`
}

type RRSet struct {
	Id      string        `json:"id"`
	Name    string        `json:"name"`
	Type    string        `json:"type"`
	TTL     *int          `json:"ttl"`
	Records []RRSetRecord `json:"records"`
	Zone    int64         `json:"zone"`
}

type RRSetRecord struct {
	Value   string `json:"value"`
	Comment string `json:"comment,omitempty"`
}

type RRSetCreateRequest struct {
	Name    string        `json:"name"`
	Type    string        `json:"type"`
	TTL     *int          `json:"ttl,omitempty"`
	Records []RRSetRecord `json:"records"`
}

type RRSetAddRecordsRequest struct {
	Records []RRSetRecord `json:"records"`
	TTL     *int          `json:"ttl,omitempty"`
}

type RRSetRemoveRecordsRequest struct {
	Records []RRSetRecord `json:"records"`
}

type ErrorResponse struct {
	Error ErrorDetail `json:"error"`
}

type ErrorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}
