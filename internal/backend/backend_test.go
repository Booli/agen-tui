package backend

// Compile-time assertion: LocalBackend must implement Backend.
var _ Backend = LocalBackend{}
