package invx

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
)

const contentType = "application/json"

// buildStringToSign assembles the exact canonical string InnovestX signs.
// Order is load-bearing; method is uppercased and host lowercased by the caller.
func buildStringToSign(apikey, method, host, path, query, ct, uid, ts, body string) string {
	return apikey + method + host + path + query + ct + uid + ts + body
}

// sign returns the lowercase hex HMAC-SHA256 of s keyed by secret.
func sign(secret, s string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(s))
	return hex.EncodeToString(mac.Sum(nil))
}
