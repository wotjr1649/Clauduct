# V2 호환성 — 현재 / 목표 / 미지원

## 0. 이 문서의 범위

**Go 코드가 존재하지 않는다.** 따라서 V2 열에 "지원"은 한 줄도 없다. 이 문서가 지금 기록할 수 있는 것은 (a) 기준선이 무엇을 지원하는지, (b) V2가 의도적으로 **다르게** 할 것이 무엇인지, (c) 무엇을 아직 모르는지 셋뿐이다.

기준선의 기능 대조표를 여기에 복제하지 않는다. 그것은 [native 기능 전수 대조](../native-feature-support.md)가 소유하며, 요청 표면·beta·hooks·설정 소스의 현행 값은 그 문서를 본다. 아래는 **V2가 그 표와 달라지는 자리**만 적는다.

## 1. V2가 의도적으로 바꾸는 것

| 영역 | 기준선 (Node V1) | V2 목표 | 근거 |
|---|---|---|---|
| 기본 실행의 설정 주입 | `--settings`로 `env`·`modelPicker`·`hooks` 3키 주입 | **주입 0키** | D10, V2-03 |
| modelPicker | `replaceBuiltInOptions: true` — 선택지 전체 교체 | 교체하지 않음 | [DECISION.md](DECISION.md) 2.5 |
| agent 정의 | `--agents`로 14개 주입 (`generalAgentModels` 14행 실측) | 기본 0개, overlay에서만 | CAP04 |
| hooks | `SubagentStart`·`SubagentStop`·`PostToolUse` 3이벤트에 Node 명령 등록 | 기본 경로에 없음 | [DECISION.md](DECISION.md) 2.3 |
| 역할별 effort 배정 | hook 등록된 자식에 역할별 effort 적용 | **잃는다.** Claude가 요청한 모델과 그 모델 기본 effort를 쓴다 | 아래 2장 |
| `--settings` `--agents` `--setting-sources` `--system-prompt` | 차단 | 차단 사유 소멸 → 전달 | [DECISION.md](DECISION.md) 2.6 |
| `--bare` `--safe-mode` | 차단 (hook이 사라져 라우팅 붕괴) | hook이 없으므로 차단 사유 소멸 | CAP10 |
| 제품 상태 위치 | `<repo>/.clauduct-status/` (설치 디렉터리 안) | OS 사용자별 state 경로 | V2-01 |
| `review-diff` 도구 명령 | `<node.exe> <repo>/src/review-diff.mjs <target>` | Go 하위 명령 또는 미지원 — **미결정** | V2-02, [DECISION.md](DECISION.md) 8장 |
| agent registration endpoint | `POST /clauduct/agents`, 기본 실행에 필수 | overlay 전용 | [ARCHITECTURE.md](ARCHITECTURE.md) 6장 |
| 비-Windows | 없음 (`src/runtime-paths.mjs`는 Windows 전용) | 신규 설계. 이관 항목 아님 | V2-04 |

## 2. 가장 큰 알려진 delta — 역할별 effort 배정

V2 기본 모드는 subagent 역할 등록을 하지 않으므로 역할별 effort 자동 배정을 잃는다. 이것은 추측이 아니라 기준선에서 이미 관측 가능한 경로다.

```text
src/native-gateway.mjs:347   등록 없는 요청 → 카운트하고 그대로 upstream으로 진행
src/clauduct.mjs:343         1회 알림: 역할별 배정 미적용, Claude가 요청한 모델과
                             그 모델 기본 effort 사용, hook 신뢰/설정은 자동 변경하지 않음
```

즉 **요청이 실패하지 않는다.** 분류는 `REGRESSION`이 아니라 `EXPECTED_DELTA`이며, 역할별 배정이 필요한 사용자는 overlay를 켠다. overlay를 끈 상태가 정상적인 제품 모드다.

## 3. V2 목표 사용례의 현재 장애물

핸드오프 13.1절이 목표로 드는 네 가지는 **코드가 아니라 정책이 막고 있다.**

| 사용례 | 현재 | V2에서 열리려면 |
|---|---|---|
| `--permission-mode plan` | 차단 (정책 범주 E) | 사용자 승인 |
| `--mcp-config .\mcp.json` | 차단 (정책 범주 E). 프로젝트 `.mcp.json` 자동 발견은 지원됨 | 사용자 승인 |
| `--plugin-dir .\plugin` | 차단 (정책 범주 E) | 사용자 승인 |
| `--worktree experiment` / `-w` | 차단 (정책 범주 E) | 사용자 승인 |

pass-through 설계(D05)를 택하면 자동으로 열리므로 구현 작업이 아니다. 열 것인지가 사용자 결정이며 [DECISION.md](DECISION.md) 8장 질문 2다.

반대로 **열지 않는 것**은 cloud/remote/download 계열 9개다(`--cloud` `--environment` `--teleport` `--remote-control` 외). 이들은 이름이 아니라 native `--help` 원문이 cloud·remote·download를 명시한 근거로 분류됐으므로, "이름만으로 차단하지 말라"는 보정의 대상이 아니다. 근거는 [옵션 분류](../claude-option-classification.md) 3장.

## 4. V2가 아직 아무것도 말할 수 없는 것

아래는 전부 **미검증(NOT_RUN)**이며 지원 선언에 포함하지 않는다. Go 구현이 없기 때문이지, 불가능해서가 아니다.

스트리밍 대화, 도구 왕복, ToolSearch·지연 로딩, 턴 중 도구 변경, 이미지·문서 입력, thinking/reasoning 재생, effort 지정, 로컬 MCP, 비대화형 실행·세션 재개, 프롬프트 캐시 지시, `context_management`, hosted search, WebFetch, structured output, count_tokens, `/v1/models` discovery, plugin·skill·user hook 발견, worktree, permission 거부, 취소·재시도, Windows 프로세스 트리 정리, 동시 세션 격리.

각 항목이 "지원"으로 올라가려면 [VALIDATION.md](VALIDATION.md)의 해당 test ID가 실행되고 증거가 붙어야 한다. **특정 `C` 게이트를 통과하지 못한 기능을 "모두 지원"에 포함하지 않는다.**

## 5. 제3자 구현이라는 사실

Claude 공식 문서는 gateway를 통한 non-Claude 모델 라우팅을 **공식 지원하지 않는다고 명시**한다. V2는 제3자 호환 구현이며 "Anthropic 공식 지원"이나 "전체 기능 100% 보장"으로 설명하지 않는다. 지원 조합(native Claude 버전 × Codex 버전 × OS)을 고정해 기록하고, drift 진단을 남긴다.

기준선이 검증한 조합은 Claude 2.1.272 / Codex 0.154.0 / Node 24.19.0 / Windows 11 10.0.26200 amd64다. V2의 조합은 아직 없다.
