// Package fs provides the filesystem contracts and virtual filesystem backends
// used by gbash, including the default mutable in-memory backend and the
// experimental read-mostly trie-backed backend.
//
// This is a supported public extension package for callers that need to supply
// custom filesystem implementations or factories. Most embedders should still
// prefer the root `github.com/ewhauser/gbash` package and its higher-level
// filesystem helpers.
package fs
