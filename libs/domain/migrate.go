package domain

import "gorm.io/gorm"

// Migrate 执行 GORM 自动迁移，创建/更新所有核心表。
func Migrate(db *gorm.DB) error {
	return db.AutoMigrate(
		&User{},
		&Question{},
		&Mistake{},
		&Image{},
		&Category{},
		&QuestionCategory{},
		&RecognitionTask{},
	)
}
