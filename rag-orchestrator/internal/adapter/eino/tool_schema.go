package eino

import (
	"encoding/json"
	"strings"

	"github.com/cloudwego/eino/schema"
)

func toolParamsForName(name string) map[string]*schema.ParameterInfo {
	switch name {
	case "tag_cloud_explore":
		return map[string]*schema.ParameterInfo{
			"topic": {Type: schema.String, Desc: "Topic to explore", Required: true},
		}
	case "articles_by_tag", "search_recaps":
		return map[string]*schema.ParameterInfo{
			"tag_name": {Type: schema.String, Desc: "Tag name to search for", Required: true},
		}
	case "date_range_filter", "related_articles", "tag_search", "extract_query_tags":
		fallthrough
	default:
		return map[string]*schema.ParameterInfo{
			"query": {Type: schema.String, Desc: "Search query or input", Required: true},
		}
	}
}

func defaultToolArgName(name string) string {
	switch name {
	case "tag_cloud_explore":
		return "topic"
	case "articles_by_tag", "search_recaps":
		return "tag_name"
	case "date_range_filter", "related_articles", "tag_search", "extract_query_tags":
		fallthrough
	default:
		return "query"
	}
}

func parseToolCallArguments(arguments string) map[string]any {
	arguments = strings.TrimSpace(arguments)
	if arguments == "" {
		return map[string]any{}
	}

	var out map[string]any
	if err := json.Unmarshal([]byte(arguments), &out); err == nil {
		return out
	}

	return map[string]any{"query": arguments}
}
