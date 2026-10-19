# Claude Code native 지원 범위

이 문서는 Windows의 Clauduct 기본 경로와 Claude Code native2.1.302에서 확인한 범위를 설명한다. 제품은 설치된 native를 실행하며 사용자 설정·승인·도구 실행과 UI는 native가 담당하고, 모델 요청의 변환·Codex 인증·응답 무결성은 Clauduct가 담당한다. 새 native와 모든 plugin/MCP 조합의 성공은 자동 승계하지 않는다. 현재 검증 결과와 남은 제약은 [호환성 문서](COMPATIBILITY.md)에 있다.

## 지원 경로

- 로컬 Windows native의 foreground 대화, print, resume·fork, Agent View와 background dispatch·attach·stop·rm을 사용한다. 저장된 Claude apps gateway provider 상태가 없고, 조직의 강제 routing 정책이 Clauduct와 충돌하지 않아야 한다. 일반 Claude 구독 OAuth와 standalone native 병행은 지원 범위다.
- native가 제공한 ordinary callable 이름·schema·인자·결과를 데이터로 보존한다. 새 도구 이름을 이유로 기존 이름으로 바꾸거나 자동 대체하지 않는다. native 도구 목록, 사용자 deny·ask 규칙, native tool hook과 각 MCP 서버 권한이 계속 적용된다.
- Clauduct가 생성하는 worker 역할은 부모의 native 도구 집합을 상속한다. 고정 whitelist로 WebFetch 검색이 막히던 결함을 수정했으며, 사용자 `--agents`의 명시적 제한·빈 배열·메뉴 대체는 유지한다. 도구 노출은 실행 승인과 별개이며 deny·ask가 계속 적용된다.
- model과 effort는 실제 계정 목록과 허용 경로로 검증한다. 모르는 이름·지원하지 않는 effort·은퇴 모델은 이유를 표시하고 거부한다. 후속 모델 안내는 자동 대체 실행이 아니다.
- 명시적으로 같은 Reply를 다시 보내면 새 제출이다. 같은 제출의 복원·재시도와 빈 wake가 중복 효과를 만들지는 않아야 한다.
- routing 증명이 깨지면 그 gateway 도메인이 종료된다. 서로 독립적으로 살아 있어야 하는 작업에는 독립 gateway 자격을 사용한다.
- 사용자 config·MCP·Skills·기록을 공유한다. 같은 native config의 Agent View 목록도 공유하며, Clauduct와 standalone의 목록을 서로 숨기는 profile 분리는 제공하지 않는다.

## 도구·UI별 조건과 대체 경로

| 기능 | 현재 지원 상태와 원인 | 사용할 경로 |
|---|---|---|
| Read·파일·shell 도구, Agent, Skill, ToolSearch 및 ordinary MCP 도구 | native가 해당 세션에 실제로 제공한 경우 bridge가 일반 callable로 전달한다. 개별 외부 도구의 서비스·권한·모든 버전을 검증했다는 뜻은 아니다 | native의 도구 목록과 deny·ask·hooks를 사용한다 |
| TaskCreate·TaskGet·TaskList·TaskUpdate | GPT 모델 이름에 대한 native 기본 도구 선택과 background 여부에 따라 제공이 달라진다. bridge가 제거하는 도구가 아니다 | 필요하면 사용자가 CLAUDE_CODE_ENABLE_TODO_TOOLS=1 또는 native의 allowedTools/tools 설정을 선택한다. Clauduct가 강제로 켜지는 않는다 |
| PowerShell | native의 OS·계정·flag·Bash deny 규칙에 따라 제공이 달라진다 | native가 지원하는 CLAUDE_CODE_USE_POWERSHELL_TOOL=1과 PowerShell 권한 규칙을 사용한다. Bash만 검사하는 사용자 hook은 PowerShell에도 맞게 설정한다 |
| Monitor | native의 버전·기능 노출 조건을 따른다. 현재 모든 flag/계정 조합의 실행을 검증하지 않았다 | 세션의 실제 제공 여부를 확인한다. 미검증을 지원 완료로 표시하지 않는다 |
| TaskOutput·옛 Task 이름 | 현행 native가 제거하거나 이름을 바꾼 옛 표면은 강제로 복원하지 않는다 | 현행 native의 Read·Agent 등 해당 기능을 사용한다 |
| Agent View 입력창의 dispatch별 /effort | 고정 native302에 없는 입력창 동작이다. GPT backend는 native의 명령 parser나 UI를 바꾸지 않는다 | clauduct agents --model … --effort …, clauduct --bg --effort … 또는 지원되는 settings를 사용한다. 같은 native에서 네 기본값 조합을 검증한다 |
| Windows warm spare | native 2.1.302·2.1.303은 `I()==="windows"` 코드 가드로 warm pool 생성 전에 반환한다(CODE_INFERRED). 실행된 warm 성공으로 보고하지 않는다 | 지원되는 cold dispatch와 복원을 사용한다. 다른 native에서 warm이 생기면 별도 검증한다 |
| teammate split pane·resume·print | native의 터미널·세션 수명 제약이 있다. Windows Terminal split pane과 teammate 복원을 bridge가 새로 구현하지 않는다 | 지원되는 in-process/일반 대화 경로를 사용한다. /resume은 native가 teammate를 복원하는 보장이 아니다 |
| Artifact·RemoteTrigger·SendUserFile·PushNotification·ShareOnboardingGuide의 Anthropic-hosted 부분 | Claude 계정·원격 서비스·요금제의 기능이다. Codex 모델 추론 자격으로 해당 서비스 권한이 생기지 않는다 | 로컬 파일 출력, 사용자가 승인한 별도 외부 연동을 사용한다. Anthropic cloud 기능의 동등 구현은 제공하지 않는다 |
| /usage·/cost·Claude 계정 관리 | Claude 구독 통계와 Codex backend 계수는 서로 다른 서비스다. Clauduct 안의 login/logout 등 계정 변경은 거부한다 | Clauduct의 사용량·진단과 standalone native의 계정 관리를 사용한다 |
| Chrome·Desktop·Remote Control·Office 등 외부 앱/서비스 조합 | Chrome과 임의 외부 조합은 미검증이며 Desktop 실행은 명시적으로 거부한다. GPT 모델 지원과 외부 앱의 backend·인증 지원은 별도다 | 지원을 확인한 로컬 CLI와 개별 연동을 사용한다. 미검증을 불가능으로 단정하지 않는다 |
| Workflow | 검증된 inline·scriptPath·named source와 제한된 계획 adapter를 지원한다. 임의 JavaScript나 출처 없는 재개는 허용하지 않는다 | 실제 Read 결과와 native 권한을 통과한 source만 사용한다 |

역할의 tools 생략과 권한 상속은 [native subagent 문서](https://code.claude.com/docs/en/sub-agents)를 따른다. Task·PowerShell·cloud 도구 조건은 [native 도구 문서](https://code.claude.com/docs/en/tools-reference), dispatch 설정은 [Agent View 문서](https://code.claude.com/docs/en/agent-view), split pane과 복원 제약은 [agent teams 문서](https://code.claude.com/docs/en/agent-teams)를 따른다. 이들은 도구의 노출·서비스 조건 근거이며 해당 기능의 Clauduct 실제 실행 PASS를 대신하지 않는다.

## GPT 모델을 사용해도 남는 native 경계

| 결정을 내리는 층 | 남는 제약의 원인 | Clauduct가 적용하는 처리 |
|---|---|---|
| native의 UI·명령 parser·OS 분기 | 입력창 문법과 Windows warm pool은 모델 요청 이전에 native가 결정한다 | 지원되는 CLI/settings와 cold dispatch를 검증한다. native 자체를 패치하거나 존재하지 않는 UI 동작을 성공으로 표시하지 않는다 |
| native의 tool 노출·사용자 권한 | 모델 이름·세션 종류·계정 flag·deny/ask에 따라 도구가 제공되거나 거부된다 | 제공된 ordinary 도구를 보존하고, 필요한 native opt-in과 권한 조건을 문서화한다. 사용자 deny를 풀지 않는다 |
| 외부 서비스의 인증·실행 | Codex 추론 권한은 Anthropic-hosted 서비스의 인증이나 실행 자격이 아니다 | 지원되는 로컬 파일·CLI 경로를 제공한다. 다른 서비스의 기능을 모델 응답만으로 구현됐다고 하지 않는다 |
| backend protocol과 bridge | 두 API의 의미가 다르거나 아직 보존 adapter가 없는 필드가 있다 | 의미를 검증한 변환만 적용한다. 명시적 실패와 미지원 설명을 남기며 조용히 값을 버리지 않는다 |

## protocol과 보안 경계

native의 도구 목록에 있는 기능도 대응 backend 의미가 없으면 같은 결과를 보장할 수 없다. temperature/top_p, thinking budget, metadata, cache breakpoint, pause_turn, 도구 결과 이미지/PDF의 명시적 warmup 계수는 현행 backend와 일대일 대응하지 않는 제약이 있다. stop_sequences는 로컬 절단이며 backend 비용·지연을 줄인다는 보장이 아니다. refusal·annotation·citation의 모든 형태를 보존하는 adapter가 없을 때는 조용한 손실 대신 명시적 실패로 처리한다. 실패 차단을 해당 기능 지원으로 부르지 않는다.

기본 신규 세션은 같은 native의 `auth status --json`으로 저장 provider 상태를 먼저 확인한다. native가 저장된 Claude apps gateway를 보고하면 `NATIVE_STORED_GATEWAY`, 상태를 확인하지 못하면 `NATIVE_PROVIDER_STATUS_UNVERIFIED`로 모델 세션 시작 전에 거절한다. 사용자는 Clauduct를 끝낸 뒤 standalone Claude Code에서 gateway 로그인을 해제하고 다시 실행할 수 있다. Clauduct가 credential 파일을 직접 읽거나 자동 로그아웃하지 않는다. 일반 first-party OAuth 상태는 거절 대상이 아니다. 충돌하는 관리 routing 정책과 사용자가 전달한 parent routing 정책도 시작 전에 거절한다.

이 시작 검사는 실행 중 공유 credential 변경이나 기존 supervisor가 보유한 provider 상태까지 인증하지 않는다. 기존 resident 복원은 새 모델 프로세스를 시작하지 않고 기존 routing proof와 reconnect 기한을 사용한다. 따라서 모든 사용자 환경에서 잘못된 목적지 송신0 또는 강한 provider certification을 보장하지 않으며, 검사를 지난 뒤 provider가 바뀌는 경로에는 잔여 위험이 있다. 자동 certification·cache·pre-warm은 제공하지 않는다. HOST_MANAGED/GATEWAY_ENV_COMPAT 전략의 background 제한을 일반 기본 경로의 미지원으로 옮겨 쓰지 않는다.

native의 parent provider 제한을 자동 주입하면 기존 HKCU 권한 정책이 선택되지 않을 수 있고, host-managed flag는 사용자 model 설정도 무시하므로 사용하지 않는다. 기존 관리자·사용자 정책을 덮어쓰지 않고 충돌을 거절하는 방향을 적용한다. provider 제어와 관리 정책의 우선순위는 [native managed settings](https://code.claude.com/docs/en/managed-settings), 상태 조회 계약은 [native CLI reference](https://code.claude.com/docs/en/cli-reference)를 따른다.

`auth status`는 offline 검사가 아니다. 저장 gateway가 있는 합성 환경에서 native 초기화의 TLS 연결 시도가 관측됐다. 그 실행에서 HTTP handler 요청과 추론은 0이었지만, 이를 모든 송신·credential refresh·상태 갱신이 없다는 보장으로 확대하지 않는다. query의 native 프로세스 세대와 모델 세션의 세대는 별도로 기록한다.

HKLM provider pin·TxR·고정 broker를 사용하는 enterprise hardening은 현재 제품에서 제공하지 않는다. PC 전체 standalone native의 정책과 endpoint에도 영향을 주므로 기본 로컬 bridge와 별도 설계가 필요하다. 준비 시험의 미실행은 이 기능이 검증됐다는 뜻이 아니다.

## 검증과 시간 예산

사용자·plugin hook의 timeout과 생략된 설정은 보존하고 native가 집행한다. Clauduct가 생성한 bridge hook에만 외부30초를 명시한다. 이는 native UserPromptSubmit 기본값을 참고한 제품 설정이며, 모든 native event의 기본값이30초라는 뜻은 아니다.

제품 hook의 공유 작업 예산14초는 확인된 순차 I/O 상한의 합이다: link 조회3초 + reconnect 대기2초 + link 재조회3초 + routing proof3초 + 등록3초. proof 실패 뒤의 mismatch 보고는 등록을 대신하며, 등록의503 재시도는 같은 HTTP3초를 공유한다. stdin 대기·해석·단계 전환도 전체 예산을 소비하므로 각 단계는 공유 기한과 자기 기한 중 먼저 오는 시각에 끝난다. reconnect2초는 호출자의 대기 제한이며 복원 프로세스 전체 수명을 뜻하지 않는다.

끝나지 않는 stdin도 공유 기한에 종료하며, 취소 뒤 입력이 도착해도 후속 hook 작업을 시작하지 않는다. 입력 reader의 회수는 일회성 hook 프로세스의 종료에 의존한다. 외부30초와 내부14초의 차이는 임의의 OS 시작·종료 지연을 보장하지 않는다. 기존 event별 거절과 native 실패 정책은 유지하며, 사용자·plugin hook에 제품 제한이나 onFailure 정책을 일괄 적용하지 않는다. 설정 형식과 event별 기본값은 [native hooks 문서](https://code.claude.com/docs/en/hooks#common-fields)를 따른다.

개발 중에는 변경 영향 시험과 유효한 기존 근거를 사용한다. 전체 관문은 push/출하 전에 새 후보에서 실행하고, 전체 exit0과 요구별 근거 충족을 각각 확인한다. 선택형 시험·지원하지 않는 native 기능·과거 실패는 원래 상태와 이유를 남긴다. 미해결 결함을 SKIP으로 바꾸지 않는다. 최종 태그 자산·설치·업데이트·제거 확인은 출하 요청 시 별도 절차로 수행한다.
