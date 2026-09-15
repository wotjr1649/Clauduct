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
| 차단 옵션 수 | 30 | **2** (권한 해제 계열만) | 아래 3장 |
| `review-diff` 헬퍼 | gateway가 주입·검증 | **미지원** (WP07까지) | 아래 3.1 |
| 제품 상태 위치 | `<repo>/.clauduct-status/` (설치 디렉터리 안) | OS 사용자별 state 경로 | V2-01 |
| `review-diff` 도구 명령 | `<node.exe> <repo>/src/review-diff.mjs <target>` | **미지원으로 확정 (WP07 재검토)** | V2-02, 아래 3.1 |
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

## 3. 옵션 정책 — 확정됨

**V2 제품 런처는 native 옵션 2개만 거부하고 나머지는 전부 전달한다.** 기준선은 30개를 차단했다.

| 거부 | 이유 |
|---|---|
| `--dangerously-skip-permissions` | 세션 전체의 권한 검사를 끈다. 이후 어떤 단계도 되돌릴 수 없다 |
| `--allow-dangerously-skip-permissions` | 같음 |

나머지 28개는 전달된다. 핸드오프 13.1절이 목표로 든 네 가지가 여기 포함된다.

| 사용례 | 상태 |
|---|---|
| `--permission-mode plan` | **전달됨** |
| `--mcp-config <path>` | **전달됨** |
| `--plugin-dir <path>` | **전달됨** |
| `--worktree <name>` / `-w` | **전달됨** |
| `--settings` `--agents` `--setting-sources` `--system-prompt` | **전달됨** — wrapper가 더 이상 그 flag를 쓰지 않으므로 차단 사유가 소멸했다 |
| `--bare` `--safe-mode` | **전달됨** — 기본 경로에 hook이 없으므로 차단 사유가 소멸했다 |
| `--restricted` `--betas` `--prompt-suggestions` `--autocompact` `--fallback-model` | **전달됨** |
| `--cloud` `--environment` `--teleport` `--remote-control` `--from-pr` `--file` `--plugin-url` 등 | **전달됨** |

마지막 행은 기준선과 크게 다르므로 이유를 적는다. **거부에는 parser가 필요하다.** V2 제품 런처에는 parser가 없고, pass-through는 코드의 부재다. 기준선의 차단 목록은 값을 먹는 옵션을 추적해야 했고 바로 그 지점에서 21개 옵션의 값을 놓쳐 `--name --model`을 혼동한 이력이 [옵션 분류](../claude-option-classification.md) 8장에 남아 있다. 거부하는 두 옵션은 값을 먹지 않아 인자 단위 정확 일치만으로 충분하고, 따라서 그 위험을 들이지 않는다.

cloud/remote 계열을 전달한다는 것은 **그 기능이 동작한다는 뜻이 아니다.** 그 요청들이 모델 추론을 필요로 하면 이 gateway를 거치고, 지원하지 않으면 명확한 capability 오류로 실패한다. native가 자기 네트워크로 처리하는 부분은 §15.4가 말하는 별도 범주이며 Go V2는 OS 수준 egress sandbox가 아니다.

알려진 오탐: 어떤 옵션의 **값이 정확히** 거부 대상 이름이면(`--append-system-prompt --dangerously-skip-permissions`) 거부된다. 문자열을 포함하기만 하는 프롬프트는 전달된다. 과잉 거부는 메시지로 드러나 복구할 수 있고 반대 방향은 그렇지 않다는 판단이다.

## 3.1 `review-diff` — WP07까지 미지원

기준선의 `src/review-diff.mjs`(56줄)는 `/code-review`가 파일 하나를 볼 때 gateway가 주입하는 검증된 diff 헬퍼다. **V2는 현재 이것을 지원하지 않는다.**

`/code-review` 자체는 계속 동작한다 — 모델이 native Bash로 직접 diff를 모은다. 사라지는 것은 기준선이 덧댄 경로 탈출 방지·2 MiB 상한·git 환경 격리·결과 형식 검증이며, 그 결과 수준은 **plain claude와 같다**. C 게이트 capability이고 WP03–WP06 어디에도 필요하지 않다.

되살릴 조건: 실사용에서 파일 단위 `/code-review`가 실제로 쓰이는 것이 관측되면 WP07에서 다룬다. 그때의 형태는 Go 헬퍼 명령(`clauduct-dev review-diff <target>`)이다 — 제품 런처에는 넣을 수 없다. bare-word 하위 명령은 프롬프트 첫 단어와 구분되지 않으며, 기준선 문서가 같은 이유로 `auth`·`install`을 차단하지 못한다고 적어 두었다.

## 4. V2가 아직 아무것도 말할 수 없는 것

아래는 전부 **미검증(NOT_RUN)**이며 지원 선언에 포함하지 않는다. Go 구현이 없기 때문이지, 불가능해서가 아니다.

스트리밍 대화, 도구 왕복, ToolSearch·지연 로딩, 턴 중 도구 변경, 이미지·문서 입력, thinking/reasoning 재생, effort 지정, 로컬 MCP, 비대화형 실행·세션 재개, 프롬프트 캐시 지시, `context_management`, hosted search, WebFetch, structured output, count_tokens, `/v1/models` discovery, plugin·skill·user hook 발견, worktree, permission 거부, 취소·재시도, Windows 프로세스 트리 정리, 동시 세션 격리.

각 항목이 "지원"으로 올라가려면 [VALIDATION.md](VALIDATION.md)의 해당 test ID가 실행되고 증거가 붙어야 한다. **특정 `C` 게이트를 통과하지 못한 기능을 "모두 지원"에 포함하지 않는다.**

## 5. 제3자 구현이라는 사실

Claude 공식 문서는 gateway를 통한 non-Claude 모델 라우팅을 **공식 지원하지 않는다고 명시**한다. V2는 제3자 호환 구현이며 "Anthropic 공식 지원"이나 "전체 기능 100% 보장"으로 설명하지 않는다. 지원 조합(native Claude 버전 × Codex 버전 × OS)을 고정해 기록하고, drift 진단을 남긴다.

기준선이 검증한 조합은 Claude 2.1.272 / Codex 0.154.0 / Node 24.19.0 / Windows 11 10.0.26200 amd64다. V2의 조합은 아직 없다.
