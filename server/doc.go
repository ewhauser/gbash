// Package server exposes a shared JSON-RPC server mode for gbash runtimes.
//
// The package is intentionally small and host-oriented. It lets embedders and
// shipped CLIs serve persistent gbash sessions over JSON-RPC endpoints without
// committing to a public client SDK.
package server
