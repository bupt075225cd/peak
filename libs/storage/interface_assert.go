package storage

// 编译期断言：确保 LocalStorage 与 S3Storage 都实现 FileStorage 接口。
var (
	_ FileStorage = (*LocalStorage)(nil)
	_ FileStorage = (*S3Storage)(nil)
)
