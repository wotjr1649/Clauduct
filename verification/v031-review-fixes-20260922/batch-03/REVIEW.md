# batch-03 직접 리뷰와 지적 처분

**최종 판단: 변경 범위 회귀 PASS, 전체 HOLD.**
기준은 원본 Claude run-01/CODEX-ASSESSMENT.md와 이번 batch.patch다.
이 문서는 Claude 재리뷰 결과가 아니라 현재 후보에 대한 Codex 직접 검토다.

검토한 공유 경계는 delegation route의 caller/metadata/Workflow 증거,
agentSelection의 모델·effort 검증과 feature 집계, settings/roleCLI의 공통 인자 경계,
역할 namespace·우선순위, count/생성의 compaction 분류, 실제 backend 검수기와 CI다.
이번 diff 19개 파일을 원본 backup과 대조했다. 전송·재전송 ledger는 batch-02 변경과
이번 실제 trace를 다시 읽었다. 이 작업을 v0.3.1의 모든 상태 조합에 대한 새 전수 리뷰로
표현하지 않는다.

**직접 리뷰에서 수정한 문제**

| 발견 | 원인 | 수정 / 판별 근거 |
|---|---|---|
| 중첩 일반 Agent의 같은 이름 충돌 | Workflow 대체 조회는 root만 가능한데 parent가 있는 일반 Agent에도 호출 | root 조건 추가, root/nested sidecar 지연 회귀 PASS |
| 무관 Workflow 손상이 일반 Agent 차단 | 대체 경로 조회 오류를 자기 sidecar의 미도착보다 먼저 반환 | 검증된 대체 결과만 채택. 본인 metadata 검증·1초 상한 유지. 수정 제거 시 `mutation-unrelated.txt` assertion FAIL |
| SDK 검증의 완료 오인 | SDK completion notification과 실제 자식 보고 수신을 동일시 | gateway parent_received=2와 실제 파일·tool transcript를 함께 판정. 제품 완료 조건을 mock하지 않음 |
| 최초 실호출 오류 유실 | probe가 native 최종 오류만 검사하고 실패 종료 시 제품 Recent 기록을 버림 | 성공·실패 종료 모두 고정 category·status·stage·StreamEnd 기록. 이전 502는 소급 해결 표시하지 않음 |
| TUI 종료 제어 실패 | stdin 중계, PowerShell flag 파싱, ANSI 줄바꿈 및 입력 제어 지연 | 실제 PTY의 검사 exe, 인용된 인자, 짧은 계측 마커와 연속 제어. 기존 5분 상한·transcript 검수 유지 |

**원본 confirmed 18건의 현재 처분**

| ID | 현재 판단 |
|---|---|
| A2-01 | batch-01 수리. 현재 전체 일반·race에서 회귀 유지 |
| A2-02 | batch-01 수리. 현재 전체 일반·race에서 회귀 유지 |
| A3-02 | batch-01 수리. 저장 장애 이후 compact 복구 회귀 유지 |
| A1-03 | batch-03 수리. pending sibling 대조, 실제 병렬 Agent/Workflow 결과 수거 PASS |
| A3-03 | batch-03 수리. 검증 출처를 거부 전에 기록하고 continuation 체크 보존 |
| INT-02 | A1-03/A3-03의 결합. 별도 독립 결함으로 중복 계수하지 않음 |
| A3-01 | batch-03 수리. optional count 분류만 일치. 일반 생성 사전 계수 없음 |
| A1-02 | batch-03 수리. compound short 값 경계와 실제 native 대조 |
| INT-05 | A1-02와 같은 수정. 알려진/모호한 옵션의 지원 범위를 문서화 |
| A1-05 | batch-03 수리. plugin namespace 내 불확실성을 보존하며 읽힌 정의와 CLI 우선순위 유지 |
| A1-01 | #42의 의도된 거부 정책 유지. 잘못 읽은 파일의 이름을 부재로 추정하는 수정은 채택하지 않음 |
| A1-06 | #48 정책상 루트 실패 거부. choiceAbsent fallback 복구 권고는 반박 상태 유지 |
| A4-01 | batch-02 제품 경로 수리·검증 유지. 현재 실제 TUI와 도구/손실 회복 PASS. 모든 외부 필터 내부 결함을 수정했다는 뜻은 아님 |
| INT-01 | batch-01/02 SSE 진단과 전송 회귀 보완 유지. 이번 probe의 최초 오류 유실 추가 보완. 별도 미분류 502가 남아 전체 무오류 판정은 HOLD |
| A5-03 | CI 태그 vet + 오프라인 검수기 추가. 로컬 대응 명령 PASS. remote CI는 실행하지 않음 |
| A5-04 | CANCELLED 누계만 누락한 대조군 추가. assertion 제거 mutation FAIL |
| A5-01 | 역사적 증거 제한 정정 완료. 원본 raw TCP source 자체는 복구 불가 |
| INT-04 | 기존 문서가 half-close를 약속했다는 전제 반박 유지. batch-02 종료 계약 문서와 현재 구현 대조 |

**원본 HOLD 5건**

| ID | 판별 / 남는 범위 |
|---|---|
| A1-20 | subcommand의 모르는 옵션 뒤 settings 경계 거부, 해당 후보가 없으면 native argv 보존. 설치 native의 mcp/plugin 공개 help 확인. 모든 subcommand 설정 쓰기를 실실행한 것은 아님 |
| A1-23 | Claude 2.1.278 공개 option arity 63개 대조 PASS. 미래 버전의 성공은 승계하지 않으며 drift 검사가 불일치를 검출 |
| A2-13 | 실제 filesystem의 .json 성공/.ready 실패·재발행, Go의 미완료 최신 receipt 거부·다음 발행 복구 PASS. 이전 turn fallback으로 통과시키지 않음 |
| A4-03 | batch-02 의도적 단절 13단계와 이번 실제 backend의 손실→재전송 차단→같은 PID 후속 작업 PASS. TUI는 별도 실제 생성·compact·Esc·복구·종료 PASS. 강제 손실 주입의 실제 backend 검증 모드는 SDK임을 구분 |
| A5-05 | nil Direct 앞의 실제 live guard가 private 형태 경로의 Execute/Count를 모두 거부함을 검사. 실제 실호출은 공개 task TEMP. guard 변경 없음 |

이 다섯 항목은 위에서 정의한 도달 조건과 지원 범위로 판정했으며, 임의 버전·모드·확장까지
지원한다는 선언으로 확장하지 않는다. 원래 18 confirmed / 5 hold를 현재 미해결 제품 결함
23개로 재사용하지 않는다.

**남은 판단 조건**

- 정상 SDK 502의 원인: 당시 최초 category가 없어 원인 확정 불가. trace는 의도적 손실 0,
  write 오류 0, 502 이후 동일 turn/step 재전송 차단을 증명한다. 실패를 없애는 제품 변경은
  이 증거만으로 선택하지 않는다. 보완된 실패 기록은 후속 재현에 사용 가능하다.
- 별도 native Claude 재리뷰: 도구 없는 bounded 요청이 6분 deadline에 결과 미수신.
  프로세스 종료·후속 부재 조회를 확인했으며, 같은 단위를 임의 재발송하지 않았다.
  직접 리뷰를 독립 Claude 승인으로 표시하지 않는다.
- 과거 raw TCP snapshot은 복구되지 않았다. 현재 제품 PASS와 분리한 문서 정정으로 관리한다.

재전송 보호를 해제하거나 자동 생성·도구 실행을 다시 수행하는 변경, 외부 필터의
설정·버전별 우회, 실패를 무시하는 assertion 변경은 적용하지 않았다.
