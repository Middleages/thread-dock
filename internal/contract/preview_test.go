package contract

import (
	"strings"
	"testing"
)

func TestPreviewRendersApprovalInformationInStableOrder(t *testing.T) {
	got := Preview(validContract())

	want := []string{
		"저장소와 기준 커밋",
		"platform/payments-api",
		"0123456789abcdef0123456789abcdef01234567",
		"전체 목표",
		"결제 실패 재시도 개선",
		"실패한 결제를 안전하게 재시도한다.",
		"중복 결제가 없다",
		"하위 작업",
		"api: 재시도 정책 구현",
		"tests: 회귀 검증",
		"작업 소유와 의존성",
		"api-builder",
		"test-builder",
		"제외 범위",
		"migrations/**",
		"검증 방법",
		"go test ./...",
		"승인 후 자동 진행",
		"승인 후 Agent가 구현·독립 확인·자동 검사·일반 변경의 기본 브랜치 반영까지 진행합니다.",
	}
	previous := -1
	for _, text := range want {
		index := strings.Index(got, text)
		if index < 0 {
			t.Fatalf("preview missing %q:\n%s", text, got)
		}
		if index < previous {
			t.Fatalf("preview is out of order at %q:\n%s", text, got)
		}
		previous = index
	}
}
