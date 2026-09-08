package controller

import "github.com/QuantumNous/new-api/model"

// Scope IDs belong to the backend storage model. Resource endpoints already
// select the account or team via authenticated request context; responses do
// not expose internal ownership IDs. Keep model JSON intact for Redis caches.
type logResponse struct {
	*model.Log
	OrgId *int `json:"org_id,omitempty"`
}

func logResponses(items []*model.Log) []logResponse {
	result := make([]logResponse, 0, len(items))
	for _, item := range items {
		result = append(result, logResponse{Log: item})
	}
	return result
}

type topUpResponse struct {
	*model.TopUp
	OrgId *int `json:"org_id,omitempty"`
}

func topUpResponses(items []*model.TopUp) []topUpResponse {
	result := make([]topUpResponse, 0, len(items))
	for _, item := range items {
		result = append(result, topUpResponse{TopUp: item})
	}
	return result
}

type midjourneyResponse struct {
	*model.Midjourney
	OrgId *int `json:"org_id,omitempty"`
}

func midjourneyResponses(items []*model.Midjourney) []midjourneyResponse {
	result := make([]midjourneyResponse, 0, len(items))
	for _, item := range items {
		result = append(result, midjourneyResponse{Midjourney: item})
	}
	return result
}

type quotaDataResponse struct {
	*model.QuotaData
	OrgId *int `json:"org_id,omitempty"`
}

func quotaDataResponses(items []*model.QuotaData) []quotaDataResponse {
	result := make([]quotaDataResponse, 0, len(items))
	for _, item := range items {
		result = append(result, quotaDataResponse{QuotaData: item})
	}
	return result
}

type subscriptionResponse struct {
	*model.UserSubscription
	OrgId *int `json:"org_id,omitempty"`
}

type subscriptionSummaryResponse struct {
	Subscription subscriptionResponse `json:"subscription"`
}

func subscriptionSummaryResponses(items []model.SubscriptionSummary) []subscriptionSummaryResponse {
	result := make([]subscriptionSummaryResponse, 0, len(items))
	for _, item := range items {
		result = append(result, subscriptionSummaryResponse{Subscription: subscriptionResponse{UserSubscription: item.Subscription}})
	}
	return result
}

type redemptionResponse struct {
	*model.Redemption
	OrgId *int `json:"org_id,omitempty"`
}

func redemptionResponses(items []*model.Redemption) []redemptionResponse {
	result := make([]redemptionResponse, 0, len(items))
	for _, item := range items {
		result = append(result, redemptionResponse{Redemption: item})
	}
	return result
}
