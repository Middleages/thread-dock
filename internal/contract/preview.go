package contract

import (
	"fmt"
	"strings"
)

const approvalProgressSentence = "승인 후 Agent가 구현·독립 확인·자동 검사·일반 변경의 기본 브랜치 반영까지 진행합니다."

// Preview renders a Markdown approval summary for a validated task contract.
func Preview(c TaskContract) string {
	var out strings.Builder

	fmt.Fprintln(&out, "# 작업 묶음 미리보기")
	fmt.Fprintln(&out)
	fmt.Fprintln(&out, "## 저장소와 기준 커밋")
	fmt.Fprintf(&out, "- 저장소: %s/%s (%s)\n", c.Repository.Owner, c.Repository.Name, c.Repository.DefaultBranch)
	fmt.Fprintf(&out, "- 기준 커밋: %s\n", c.BaseCommit)
	fmt.Fprintln(&out)
	fmt.Fprintln(&out, "## 전체 목표")
	fmt.Fprintf(&out, "### %s\n\n", c.Parent.Title)
	fmt.Fprintln(&out, c.Parent.Body)
	writeList(&out, c.Parent.AcceptanceCriteria)
	fmt.Fprintln(&out)
	fmt.Fprintln(&out, "## 하위 작업")
	for _, child := range c.Children {
		fmt.Fprintf(&out, "- %s: %s\n", child.Key, child.Title)
	}
	fmt.Fprintln(&out)
	fmt.Fprintln(&out, "## 작업 소유와 의존성")
	for _, task := range c.Tasks {
		dependencies := "없음"
		if len(task.DependsOn) > 0 {
			dependencies = strings.Join(task.DependsOn, ", ")
		}
		fmt.Fprintf(&out, "- %s: %s (%s), 브랜치 %s, 의존성 %s\n", task.ID, task.Owner, task.Role, task.Branch, dependencies)
	}
	fmt.Fprintln(&out)
	fmt.Fprintln(&out, "## 제외 범위")
	writeList(&out, c.Protected)
	fmt.Fprintln(&out)
	fmt.Fprintln(&out, "## 검증 방법")
	writeList(&out, c.Verification)
	fmt.Fprintln(&out)
	fmt.Fprintln(&out, "## 승인 후 자동 진행")
	fmt.Fprintln(&out, approvalProgressSentence)

	return out.String()
}

func writeList(out *strings.Builder, values []string) {
	for _, value := range values {
		fmt.Fprintf(out, "- %s\n", value)
	}
}
