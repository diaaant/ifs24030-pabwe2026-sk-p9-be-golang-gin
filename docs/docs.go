// Package docs menyediakan spesifikasi OpenAPI/Swagger aplikasi.
package docs

import (
	_ "embed"
	"encoding/json"
)

//go:embed swagger.json
var specJSON []byte

// GetSwaggerJSON mengembalikan spesifikasi OpenAPI dengan host & scheme yang
// disesuaikan secara dinamis dengan pemanggil, sehingga Swagger UI selalu
// menembak alamat yang benar (lokal maupun terdeploy).
func GetSwaggerJSON(host, scheme string) ([]byte, error) {
	var spec map[string]any
	if err := json.Unmarshal(specJSON, &spec); err != nil {
		return nil, err
	}
	spec["host"] = host
	spec["schemes"] = []string{scheme}
	return json.Marshal(spec)
}
