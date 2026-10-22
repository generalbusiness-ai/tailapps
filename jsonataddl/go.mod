module github.com/generalbusiness-ai/tailapps/jsonataddl

go 1.27.3

retract v0.1.0 // Its published bytes conflict with the immutable checksum database, so this version is unusable.

require (
	github.com/jsonata-go/jsonata v0.0.0-20250709164031-599f35f32e5f
	github.com/ncruces/go-sqlite3 v0.35.3
)

require (
	github.com/ncruces/go-sqlite3-wasm/v3 v3.2.35304 // indirect
	github.com/ncruces/julianday v1.0.0 // indirect
	golang.org/x/sys v0.48.0 // indirect
	golang.org/x/text v0.42.0 // indirect
)
