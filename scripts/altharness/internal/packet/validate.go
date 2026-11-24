package packet

import (
	"context"
	"fmt"
	"strings"
)

func refSchema(name string) map[string]any {
	return map[string]any{"$ref": "#/$defs/" + name}
}

func validatePacketDoc(doc *Document, schema map[string]any) ([]Issue, error) {
	var issues []Issue
	if err := validateSchemaNode(doc.Data, refSchema("TaskPacket"), schema, "$", &issues); err != nil {
		return issues, err
	}
	if scope, ok := asMap(doc.Data["scope"]); ok {
		if stop, ok := asString(scope["stop_condition"]); ok && isPlaceholder(stop) {
			issues = append(issues, Issue{
				Code: "PLACEHOLDER_VALUE", Path: "$.scope.stop_condition",
				Message: fmt.Sprintf("stop_condition must not be a placeholder (%q)", stop),
			})
		}
	}
	if policy, ok := asMap(doc.Data["execution_policy"]); ok {
		verifyCmd := policy["verification_command"]
		forbid := false
		if b, ok := policy["forbid_repo_wide_test_build"].(bool); ok && b {
			forbid = true
		}
		declared := "fix-existing"
		checkPolicy := true
		if _, present := policy["policy"]; present {
			s, ok := asString(policy["policy"])
			if !ok {
				checkPolicy = false
			} else {
				declared = s
				checkPolicy = declared == "fix-existing" || declared == "block-new"
			}
		}
		if cmd, ok := asString(verifyCmd); ok && strings.TrimSpace(cmd) != "" {
			checkVerificationCommand(cmd, "$.execution_policy.verification_command", extractPacketTargets(doc.Data), forbid, &issues)
			if checkPolicy {
				checkPolicyCommandMatch(cmd, declared, "$.execution_policy.verification_command", &issues)
			}
		}
		validateMeasuredExecution(policy, verifyCmd, &issues)
	}
	if doc.IsMarkdown {
		checkMarkdownHeadings(doc.Body, packetHeadings, &issues)
		checkPacketMarkdownBody(doc.Body, &issues)
	}
	return issues, nil
}

func validateHandoffImpact(data map[string]any, issues *[]Issue) {
	impact, ok := asMap(data["runtime_behavior_impact"])
	if !ok {
		return
	}
	if summary, ok := asString(impact["summary"]); ok && isPlaceholder(summary) {
		*issues = append(*issues, Issue{
			Code: "PLACEHOLDER_VALUE", Path: "$.runtime_behavior_impact.summary",
			Message: "runtime_behavior_impact.summary must not be empty or a placeholder",
		})
	}
	if dims, ok := asSlice(impact["checked_dimensions"]); ok {
		for idx, dim := range dims {
			if s, ok := asString(dim); ok && isPlaceholder(s) {
				*issues = append(*issues, Issue{
					Code:    "PLACEHOLDER_VALUE",
					Path:    fmt.Sprintf("$.runtime_behavior_impact.checked_dimensions[%d]", idx),
					Message: fmt.Sprintf("checked_dimensions entry must not be a placeholder (%q)", s),
				})
			}
		}
	}
}

func validateHandoffVerificationAndFiles(data map[string]any, issues *[]Issue) {
	if verification, ok := asMap(data["verification"]); ok {
		if cmd, ok := asString(verification["command"]); ok && strings.TrimSpace(cmd) != "" {
			checkVerificationCommand(cmd, "$.verification.command", nil, false, issues)
		}
		if data["status"] == "completed" {
			if code, ok := asInt(verification["exit_code"]); ok && code != 0 {
				*issues = append(*issues, Issue{
					Code: "COMPLETED_WITH_FAILED_VERIFICATION", Path: "$.verification.exit_code",
					Message: fmt.Sprintf("status='completed' requires verification.exit_code == 0 (got %d)", code),
				})
			}
		}
	}
	if modified, ok := asSlice(data["modified_files"]); ok {
		seen := map[string]struct{}{}
		for idx, entry := range modified {
			item, ok := asMap(entry)
			if !ok {
				continue
			}
			rel, ok := asString(item["path"])
			if !ok {
				continue
			}
			norm := normalizeRepoRel(rel)
			if _, dup := seen[norm]; dup {
				*issues = append(*issues, Issue{
					Code:    "DUPLICATE_MODIFIED_FILE",
					Path:    fmt.Sprintf("$.modified_files[%d].path", idx),
					Message: fmt.Sprintf("duplicate modified file path '%s'", norm),
				})
			}
			seen[norm] = struct{}{}
		}
	}
}

func validateHandoffDoc(ctx context.Context, doc *Document, schema map[string]any, packet *Document, screener []map[string]any, repoRoot string) ([]Issue, error) {
	var issues []Issue
	if err := validateSchemaNode(doc.Data, refSchema("TaskHandoff"), schema, "$", &issues); err != nil {
		return issues, err
	}
	if rationale, ok := asString(doc.Data["rationale"]); ok && isPlaceholder(rationale) {
		issues = append(issues, Issue{
			Code: "PLACEHOLDER_VALUE", Path: "$.rationale",
			Message: fmt.Sprintf("rationale must explain changes or state '変更なし' (got %q)", rationale),
		})
	}
	validateHandoffImpact(doc.Data, &issues)
	validateHandoffVerificationAndFiles(doc.Data, &issues)
	if err := validateBehaviorCandidates(ctx, doc.Data, screener, &issues, repoRoot); err != nil {
		return issues, err
	}
	allow := false
	if packet != nil {
		if scope, ok := asMap(packet.Data["scope"]); ok {
			if b, ok := scope["allow_generated_code_edits"].(bool); ok && b {
				allow = true
			}
		}
	}
	if err := validateGeneratedCodeEdits(ctx, doc.Data, repoRoot, allow, &issues, ""); err != nil {
		return issues, err
	}
	if doc.IsMarkdown {
		checkMarkdownHeadings(doc.Body, handoffHeadings, &issues)
	}
	if packet != nil {
		if err := crossValidateHandoff(doc.Data, packet, schema, &issues); err != nil {
			return issues, err
		}
	}
	return issues, nil
}

func crossValidateModifiedFiles(modified any, scope map[string]any, issues *[]Issue) {
	files, ok := asSlice(modified)
	if !ok {
		return
	}
	maxFiles, _ := asInt(scope["max_files"])
	allowed := stringList(scope["allowed_paths"])
	forbidden := stringList(scope["forbidden_paths"])
	if int64(len(files)) > maxFiles {
		*issues = append(*issues, Issue{
			Code: "MAX_FILES_EXCEEDED", Path: "$.modified_files",
			Message: fmt.Sprintf("modified_files count %d exceeds packet max_files=%d", len(files), maxFiles),
		})
	}
	for idx, entry := range files {
		item, ok := asMap(entry)
		if !ok {
			continue
		}
		rel, ok := asString(item["path"])
		if !ok {
			continue
		}
		if !matchesAnyGlob(rel, allowed) {
			*issues = append(*issues, Issue{
				Code:    "PATH_OUTSIDE_ALLOWED_PATHS",
				Path:    fmt.Sprintf("$.modified_files[%d].path", idx),
				Message: fmt.Sprintf("modified file '%s' is outside packet scope.allowed_paths %v", rel, allowed),
			})
		}
		if len(forbidden) > 0 && matchesAnyGlob(rel, forbidden) {
			*issues = append(*issues, Issue{
				Code:    "PATH_IN_FORBIDDEN_PATHS",
				Path:    fmt.Sprintf("$.modified_files[%d].path", idx),
				Message: fmt.Sprintf("modified file '%s' matches packet scope.forbidden_paths %v", rel, forbidden),
			})
		}
	}
}

func crossValidateHandoff(handoff map[string]any, packetDoc *Document, schema map[string]any, issues *[]Issue) error {
	packetIssues, err := validatePacketDoc(packetDoc, schema)
	if err != nil {
		return err
	}
	if len(packetIssues) > 0 {
		for _, issue := range packetIssues {
			*issues = append(*issues, Issue{
				Code: "PACKET_" + issue.Code, Path: "packet:" + issue.Path, Message: issue.Message,
			})
		}
		return nil
	}
	packet := packetDoc.Data
	if !strictEqual(packet["packet_id"], handoff["packet_id"]) {
		*issues = append(*issues, Issue{
			Code: "PACKET_ID_MISMATCH", Path: "$.packet_id",
			Message: fmt.Sprintf("handoff packet_id %v does not match packet %v", handoff["packet_id"], packet["packet_id"]),
		})
	}
	scope, _ := asMap(packet["scope"])
	files := handoff["modified_files"]
	if files == nil {
		files = []any{}
	}
	crossValidateModifiedFiles(files, scope, issues)
	maxFunctions, _ := asInt(scope["max_functions"])
	if modFuncs, ok := asInt(handoff["modified_functions_count"]); ok && modFuncs > maxFunctions {
		*issues = append(*issues, Issue{
			Code: "MAX_FUNCTIONS_EXCEEDED", Path: "$.modified_functions_count",
			Message: fmt.Sprintf("modified_functions_count %d exceeds packet max_functions=%d", modFuncs, maxFunctions),
		})
	}
	policy, _ := asMap(packet["execution_policy"])
	forbid := false
	declared := "fix-existing"
	checkPolicy := true
	if policy != nil {
		if b, ok := policy["forbid_repo_wide_test_build"].(bool); ok && b {
			forbid = true
		}
		if _, present := policy["policy"]; present {
			s, ok := asString(policy["policy"])
			if !ok {
				checkPolicy = false
			} else {
				declared = s
				checkPolicy = declared == "fix-existing" || declared == "block-new"
			}
		}
	}
	if verification, ok := asMap(handoff["verification"]); ok {
		if cmd, ok := asString(verification["command"]); ok && strings.TrimSpace(cmd) != "" {
			var required []string
			if handoff["status"] == "completed" {
				required = extractPacketTargets(packet)
			}
			checkVerificationCommand(cmd, "$.verification.command", required, forbid, issues)
			if checkPolicy {
				checkPolicyCommandMatch(cmd, declared, "$.verification.command", issues)
			}
		}
	}
	return nil
}
