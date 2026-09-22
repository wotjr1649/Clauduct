# v0.3.1 첫 묶음 — 역할 선택과 회귀 근거

2026-09-21. 기준은 `main`/`v0.3.0`의 commit
`149068edd693fb860a03244a2ea15764bcd68c34`이다.
작업 위치는 `D:\AIDEV\clauduct-v031`, 브랜치는 `fix/v031`이다.
이 기록은 로컬 개발 변경의 판정이며 v0.3.1 출시 판정이 아니다.

## 사용자 결정

| 이슈 | 결정 | 이번 묶음의 상태 |
|---|---|---|
| [#42](https://github.com/wotjr1649/Clauduct/issues/42) | 정상 정의와 우선순위를 보존하고, 손상된 정의 때문에 존재 여부를 알 수 없는 역할만 거부 | 구현·검사 완료 |
| [#45](https://github.com/wotjr1649/Clauduct/issues/45) | native 모델 선택을 보존하면서 실제 선택을 확인하고 완료 추적 유지 | 구현·검사 완료 |
| [#50](https://github.com/wotjr1649/Clauduct/issues/50) | 현행 추정과 카운터 유지, 실제 1회 관측 후 재검토 | 정책 유지; 발생을 관측했다고 주장하지 않음 |
| [#56](https://github.com/wotjr1649/Clauduct/issues/56) | 압축에만 medium 상한을 적용하고 이후 원래 effort로 복귀 | 정책 확정·구현 대기 |

## 구현한 범위

| 이슈 | 동작과 근거 |
|---|---|
| #42 | 읽지 못한 일반 역할 파일은 이름을 확정할 수 없음을 별도로 남긴다. 읽힌 정의는 기존 우선순위로 선택하고, 끝까지 정의를 찾지 못했을 때 불완전한 스캔을 단순 부재로 처리하지 않는다. [역할 스캔 검사](../../go/internal/app/role_scan_test.go) |
| [#43](https://github.com/wotjr1649/Clauduct/issues/43) | 내장 역할 표의 철자를 한 함수에서 정규화한다. 사용자 정의를 먼저 확인하며, 일반 사용자 이름과 표 밖의 메뉴 이름은 그대로 둔다. 선택·복원·재개·기록에서 같은 이름 비교를 사용한다. [라우팅 검사](../../go/internal/protocol/bridge/route_test.go), [선택·복원 검사](../../go/internal/gateway/delegation_test.go) |
| [#44](https://github.com/wotjr1649/Clauduct/issues/44) | native에서 대소문자만 다른 사용자 `Fork`가 도달 가능함을 확인했다. 사용자 역할 구분을 선택 기록과 저널에 보존해 내장 `fork`의 모델 상속·재개 규칙을 적용하지 않는다. [제품 경로 검사](../../go/internal/app/roles_test.go) |
| #45 | 모델 미지정 역할은 기존 `pending` 기록으로 완료 대기 대상에 넣는다. Agent의 모델을 부모 것으로 바꾸지 않고, 첫 native 턴의 모델·effort와 metadata를 대조한 뒤 선택을 확정·저장한다. 요청의 모델·effort 불일치와 미확인 영수증은 계속 거부한다. [선택·완료·거부 검사](../../go/internal/gateway/unrouted_role_test.go) |
| [#51](https://github.com/wotjr1649/Clauduct/issues/51) | `input_audio`를 추정할 수 없는 미디어에 포함한다. 메시지 내용과 도구 결과를 모두 검사한다. 미지의 종류를 나타내던 fixture에는 존재하지 않는 이름을 사용한다. 오디오 입력 전체의 지원을 새로 선언하는 변경은 아니다. [미디어 검사](../../go/internal/gateway/estimate_media_test.go) |
| [#59](https://github.com/wotjr1649/Clauduct/issues/59) | 하위 디렉터리의 plugin 역할 이름, 없는 projects 루트의 두 분기, 실제 `shadowed` fixture에서 낮은 우선순위 디렉터리가 쓰이는지를 검사한다. Explore에서 판별력이 없는 `fellBack` 중복 조건만 제거했고 Workflow 쪽 조건은 보존했다. [부재 분기 검사](../../go/internal/gateway/absent_tree_test.go), [역할 스캔 검사](../../go/internal/app/role_scan_test.go) |

## native 관찰

Claude Code **2.1.278**, Windows amd64, Go **1.27.1**에서 확인했다.
모델 응답은 로컬 fixture가 제공했고 실제 과금 백엔드는 호출하지 않았다.

- 브리지를 거치지 않은 native가 임시 프로젝트의 `Fork.md`를 읽고,
  부모 `gpt-5.6-luna`와 다른 `gpt-6-astra`로 요청했다.
  검사 파일은 `go/internal/app/role_case_probe_test.go`이다.
- 제품 경로의 사용자 `Fork`는 정의의 `gpt-5.6-sol/medium`과
  명시한 `gpt-5.6-terra/high`를 각각 유지했고, 두 경우 모두 부모가 결과를 받았다.
- `statusline-setup`은 native가 선택한 `gpt-5.6-terra/low`를 유지했고,
  선택 확인과 부모 결과 수신까지 관찰했다.

따라서 인계 문서의 정규화 제안 중 **hook에서 역할 이름을 무조건 정규화하는 부분은
적용하지 않았다.** 그 위치에서는 사용자 `Fork`와 내장 `fork`를 구분할 근거가 없다.

## 검사 결과

| 검사 | 결과 |
|---|---|
| `gofmt -l .` | 출력 없음 |
| `go vet ./...` | PASS |
| `CGO_ENABLED=0 go build ./...` | PASS |
| `CGO_ENABLED=0 go test -count=1 -timeout=8m ./...` | 최종 재실행 PASS; app 225.709초 |
| `CGO_ENABLED=1 go test -count=1 -timeout=8m -race ./...` | PASS; app 212.020초; 설치된 gcc 사용 |
| 마지막으로 추가한 native 선택 입력 경계 검사 | 일반 검사·race 모두 PASS |
| 사용자 역할 이름과 Workflow 실행 출처 구분 | 추가 race 검사 PASS |
| 코드 변이 | 12/12가 지정한 테스트의 assertion 실패로 검출됨; 빌드 실패를 검출로 세지 않음 |
| 문서 인용·최종 diff | 문서 116개 / 인용 89개 / 로컬 링크 611개, 실패 0; `git diff --check` PASS |

코드 변이는 Go `-overlay`로만 적용했다. 작업 파일을 되돌리거나 다른 변경과 섞지 않았다.
[검출 결과](mutations.json)에 각 assertion 실패를 보존했다. 원래 로컬 재현 입력과 실패
로그는 `.tmp/v031-mutations/`에 있다.

| 변이 | 실패한 검사 |
|---|---|
| #42 불완전한 스캔의 부재 거부 제거 | `TestUnreadableRoleWithDifferentFilenameDoesNotFallBack` |
| #43 내장 이름의 대소문자 일치 제거 | `TestCanonicalRolePreservesNamesOutsideTheBuiltinTables` |
| #44 사용자 Fork에도 내장 상속 적용 | `TestNativeCustomForkRetainsItsDefinitionAndExplicitChoice/definition` |
| #44 사용자 Fork의 명시 모델도 내장 규칙으로 거부 | `TestCustomForkIdentitySurvivesExplicitSelectionAndRestore` |
| #44 저널의 사용자 역할 구분 제거 | `TestCustomForkIdentitySurvivesExplicitSelectionAndRestore` |
| #45 부모 모델 fallback 복원 | `TestARoleWithNoRouteKeepsNativeChoiceAndIsTracked` |
| #45 native 선택의 pending 기록 제거 | `TestARoleWithNoRouteKeepsNativeChoiceAndIsTracked` |
| #45 실제 요청 effort와 native 선택 대조 제거 | `TestNativeChoiceChecksTheActualRequestEffort` |
| #51 오디오 미디어 분류 제거 | `TestReasoningIsOpaqueButIsNotMedia` |
| #59 하위 역할의 base name을 상대 경로로 변경 | `TestUnreadableNestedPluginRoleClaimsItsBaseName` |
| #59 projects 부재의 역할별 분기 우회 | `TestAbsentProjectsChoiceDistinguishesKnownAndCustomRoles` |
| #59 shadowed fixture의 낮은 우선순위 디렉터리 제거 | `TestAnUnreadableDuplicateUnderAnotherFilenameIsNotCaught` |

## 남은 묶음과 순서

| 순서 | 이슈 | 이유 |
|---|---|---|
| 다음 | #49 · #47 · #57 | 턴 영수증의 쓰기·읽기 경쟁, 재시작 상태 초기화, 세션 임시 파일 수명은 같은 생명주기 검토가 필요하다 |
| 그다음 | #46 · #48 | projects 루트 오류를 한 곳에서 분류한 뒤 부재 분기와 진단 카운터를 정리한다 |
| 이후 | #54 · #55 | argv의 값·`--` 경계를 먼저 고치고 문서의 거부 계약을 맞춘다 |
| 이후 | #53 · #58 | SessionStart 실패의 도달 경로와 native 요청 분류 지원 조건을 함께 확인한다 |
| 이후 | #56 | 확정된 압축 effort 정책을 구현하고 count_tokens와 요청 입력의 차이도 함께 확인한다 |
| 유지 | #50 · #52 | #50은 실제 관측 전 현행 유지. #52는 경로 탈출 방어를 유지하는 wontfix |

#49의 영수증 경쟁과 나머지 미해결 항목이 남아 있으므로 전체 v0.3.1 완료나 출시를
선언하지 않는다. 새 저널의 `customRole`과 `native-selection`을 v0.3.0이 읽는
다운그레이드 복원은 검증하지 않았다. 기존 저널 읽기 회귀 검사는 통과했다.

새 동작의 실제 과금 백엔드 시험과 수동 TUI 수용 검사는 하지 않았다.
Node V1·PowerShell 전체 회귀는 이번 Go 역할 묶음에서 다시 실행하지 않았다.
원격 이슈 변경, push, PR, 병합, 태그, Release, 설치본 변경은 하지 않았다.
원래 `D:\AIDEV\Clauduct`의 기존 변경과 미추적 검증 자료는 보존했다.
