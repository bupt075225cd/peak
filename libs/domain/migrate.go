package domain

import "gorm.io/gorm"

// Migrate 执行 GORM 自动迁移，创建/更新所有核心表，
// 并幂等播种应用注册表默认记录（仅表为空时写入）。
func Migrate(db *gorm.DB) error {
	if err := db.AutoMigrate(
		&User{},
		&Question{},
		&Mistake{},
		&Image{},
		&Category{},
		&QuestionCategory{},
		&RecognitionTask{},
		&Poem{},
		&AppClient{},
		// 家庭资产账本：密文业务表 + 每用户密钥盐 + 审计链 + 同意/注销留痕。
		&LedgerKeyring{},
		&LedgerItem{},
		&LedgerMember{},
		&LedgerSnapshot{},
		&LedgerAuditLog{},
		&LedgerAuditHead{},
		&LedgerConsent{},
		&LedgerDeletionRequest{},
	); err != nil {
		return err
	}
	return seedAppClients(db)
}

// seedAppClients 应用注册表幂等播种：表为空时写入默认应用，
// 已有记录（含被删除的）则不覆盖，便于运维按环境增删。
func seedAppClients(db *gorm.DB) error {
	var count int64
	if err := db.Model(&AppClient{}).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	return db.Create(&DefaultAppClients).Error
}
