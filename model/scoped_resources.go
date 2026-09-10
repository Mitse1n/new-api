package model

import (
	"github.com/QuantumNous/new-api/common"
)

func GetScopedTask(scope ResourceScope, taskID string) (*Task, bool, error) {
	var tasks []Task
	err := scope.Apply(DB).Where("task_id = ?", taskID).Limit(2).Find(&tasks).Error
	if err != nil || len(tasks) != 1 {
		return nil, false, err
	}
	return &tasks[0], true, nil
}

func GetScopedQuotaDates(scope ResourceScope, start, end int64) ([]*QuotaData, error) {
	rows := make([]*QuotaData, 0)
	err := scope.Apply(DB.Model(&QuotaData{})).Where("created_at >= ? AND created_at <= ?", start, end).
		Select("user_id, username, model_name, created_at, SUM(count) AS count, SUM(quota) AS quota, SUM(token_used) AS token_used").Group("user_id, username, model_name, created_at").Order("created_at").Find(&rows).Error
	return rows, err
}

func GetScopedFlowQuotaData(scope ResourceScope, start, end int64) ([]*FlowQuotaData, error) {
	rows := make([]*FlowQuotaData, 0)
	err := scope.Apply(flowQuotaBaseQuery(start, end)).
		Select("user_id, username, token_id, use_group, channel_id, model_name, SUM(count) AS count, SUM(quota) AS quota, SUM(token_used) AS token_used").
		Group("user_id, username, token_id, use_group, channel_id, model_name").Order("quota DESC").Find(&rows).Error
	if err != nil {
		return nil, err
	}
	if err := fillFlowTokenNames(rows); err != nil {
		return nil, err
	}
	if err := fillFlowChannelNames(rows); err != nil {
		return nil, err
	}
	if !scope.AllMembers {
		for _, row := range rows {
			row.Username, row.ChannelName = "", ""
		}
	}
	return rows, nil
}

func GetScopedTopUps(scope ResourceScope, keyword string, page *common.PageInfo) ([]*TopUp, int64, error) {
	rows := make([]*TopUp, 0)
	query := scope.Apply(DB.Model(&TopUp{}))
	if keyword != "" {
		query = query.Where("trade_no LIKE ?", "%"+keyword+"%")
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	err := query.Order("id DESC").Limit(page.GetPageSize()).Offset(page.GetStartIdx()).Find(&rows).Error
	return rows, total, err
}
