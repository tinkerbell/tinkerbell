package templates_test

import (
	"context"
	"strings"
	"testing"

	"github.com/tinkerbell/tinkerbell/ui/templates"
)

func TestWorkflowTableRowEnableAction(t *testing.T) {
	tests := []struct {
		name         string
		wf           templates.Workflow
		wantEnable   bool
		wantTooltip  bool
		wantDashOnly bool
	}{
		{
			name:       "disabled and can update shows active Enable button",
			wf:         templates.Workflow{Name: "wf-1", Namespace: "default", Disabled: true, CanUpdate: true},
			wantEnable: true,
		},
		{
			name:        "disabled and cannot update shows locked tooltip",
			wf:          templates.Workflow{Name: "wf-1", Namespace: "default", Disabled: true, CanUpdate: false},
			wantTooltip: true,
		},
		{
			name:         "not disabled shows dash regardless of permission",
			wf:           templates.Workflow{Name: "wf-1", Namespace: "default", Disabled: false, CanUpdate: false},
			wantDashOnly: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf strings.Builder
			if err := templates.WorkflowTableRow(tt.wf).Render(context.Background(), &buf); err != nil {
				t.Fatalf("Failed to render row: %v", err)
			}
			html := buf.String()

			hasActiveButton := strings.Contains(html, `hx-post="/workflows/default/wf-1/enable"`)
			hasTooltip := strings.Contains(html, "You need") && strings.Contains(html, "aria-disabled")
			hasDash := strings.Contains(html, "&mdash;")

			if hasActiveButton != tt.wantEnable {
				t.Errorf("active Enable button present = %v, want %v", hasActiveButton, tt.wantEnable)
			}
			if hasTooltip != tt.wantTooltip {
				t.Errorf("locked tooltip present = %v, want %v", hasTooltip, tt.wantTooltip)
			}
			if tt.wantDashOnly && !hasDash {
				t.Error("expected dash placeholder for a non-disabled row")
			}
		})
	}
}

func TestWorkflowDisabledControlEnableAction(t *testing.T) {
	tests := []struct {
		name        string
		wf          templates.WorkflowDetail
		wantEnable  bool
		wantTooltip bool
	}{
		{
			name:       "disabled and can update shows active Enable button",
			wf:         templates.WorkflowDetail{Name: "wf-1", Namespace: "default", Disabled: true, CanUpdate: true},
			wantEnable: true,
		},
		{
			name:        "disabled and cannot update shows locked tooltip",
			wf:          templates.WorkflowDetail{Name: "wf-1", Namespace: "default", Disabled: true, CanUpdate: false},
			wantTooltip: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf strings.Builder
			if err := templates.WorkflowDisabledControl(tt.wf).Render(context.Background(), &buf); err != nil {
				t.Fatalf("Failed to render control: %v", err)
			}
			html := buf.String()

			hasActiveButton := strings.Contains(html, `hx-post="/workflows/default/wf-1/enable"`)
			hasTooltip := strings.Contains(html, "You need")

			if hasActiveButton != tt.wantEnable {
				t.Errorf("active Enable button present = %v, want %v", hasActiveButton, tt.wantEnable)
			}
			if hasTooltip != tt.wantTooltip {
				t.Errorf("locked tooltip present = %v, want %v", hasTooltip, tt.wantTooltip)
			}
		})
	}
}
