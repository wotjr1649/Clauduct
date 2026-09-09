# OmniRoute의 OpenAI Codex 인증 — Clauduct 적용 분석

2026-09-06. 참조 대상은 `D:\AIDEV\_ref\OmniRoute`, package **3.8.51**, commit **9d1a896c6058b2ade94c9078c2e54377b9aa76d3**다. Git 작업 트리는 읽기 전후 clean이다. 아래는 로컬 소스의 정적 분석과 현재 공식 문서 대조다. OmniRoute·Codex를 실행하거나 실제 계정·저장된 인증 자료를 확인하지 않았다.

## 1. 결론

사용자 실행 진입점 후속: [user-session.mjs](../poc/user-session.mjs)는 기존 OAuth를 사용자 프로세스의 메모리에서 Codex 전송 계층으로 연결한다. 로컬 합성 44/44를 통과했지만 실제 자격증명 경로는 에이전트가 실행하지 않았다. 이번에 같은 Codex executor의 요청 변환을 추가로 읽어 빈 instructions의 보충과 max_tokens/max_output_tokens 제거를 확인했다. Clauduct의 자체 합성 검사 요청은 토큰 한도를 처음부터 만들지 않으며, 외부 클라이언트가 명시한 한도는 전송 전에 거부한다. OmniRoute의 제거 동작을 backend 지원의 실제 관찰 증거로 바꾸지 않는다.

후속 구현: 사용자의 최신 지시에 따라 [Clauduct HTTP 게이트웨이와 Codex 전송 계층](../poc/게이트웨이.md)을 구현했다. BaseExecutor의 Bearer 생성과 CodexExecutor의 workspace/account 헤더 연결을 정적으로 다시 확인해 책임 분리에 반영했다. Claude OAuth의 환경변수 저장은 사용자 보고로 확인했으며 추가 Claude 설정 조사를 게이트웨이 구현의 조건으로 두지 않는다. 합성 HTTP 검사 67/67을 통과했지만 실제 OAuth 사용·CLI 연결은 미검증이다. 아래 인증방식 목록과 기존 guard로 제한된 분석 범위는 유지한다.

**OmniRoute의 OAuth 흐름은 설계 참고로 사용하며, 첫 PoC는 기존 Codex OAuth의 사용자 운영 경로를 우선한다.** 후속 사용자 보고에서 Claude는 `CLAUDE_CODE_OAUTH_TOKEN` + MAX 20 PLAN, Codex는 로그인 OAuth를 사용한다고 확인됐다. 값·유효성·계정 상태를 에이전트가 검사한 것은 아니다. 기존 사용자 HTTP 검사도 이미 모델 응답을 받았으므로 새 PKCE 로그인은 첫 PoC의 필수 조건에서 제외한다. 기존 토큰의 복사·업로드나 OmniRoute import는 채택하지 않는다.

브라우저 PKCE와 device code는 독립 로그인이 필요해질 경우의 조건부 대안이다. 기존 사용자 실행 검사와 같은 메모리 사용·refresh/쓰기 없음·만료 시 중단 계약을 제품에 적용할지는 별도 검증해야 한다. 에이전트의 credential-path 거부는 그대로이며 이 설계 결정이 인증 접근 권한을 추가하지 않는다.

인증을 바꿔 헤더 누락을 해결할 근거는 없다. 사용자가 실행한 Node https/fetch/.NET은 이미 HTTP 200과 정확한 모델 응답을 받았다. 실패 지점은 Content-Type 계약이며, 잘못된 자격증명이라는 증거가 아니다. 동일 목적의 추가 라이브 검사는 종료한다.

## 2. 확인한 경로와 적용 판단

| 경로 | OmniRoute에서 확인한 동작 | Clauduct 적용 판단 |
|---|---|---|
| 브라우저 OAuth Authorization Code + PKCE | `providers/codex.ts`: authorization_code_pkce, callback port 1455, `/auth/callback`, state·code_challenge·verifier 사용. code를 token endpoint에서 교환하고 access/refresh/id token과 계정 메타데이터를 매핑. 공통 exchange route가 연결을 저장 | **독립 로그인이 필요할 때의 후보.** 초기 분석에서는 우선 후보였으나 사용자 기존 OAuth 확인 후 첫 PoC의 필수 구현에서 제외했다. 새 로그인 구현·실행은 하지 않음 |
| Device code | `codexDeviceFlow.ts`: usercode 발급 → 사용자 기기 승인 → polling으로 authorization_code/verifier 수신 → token 교환. connect 페이지의 사용자 브라우저가 수행하고 연결 완료 API에 전달·저장 | callback을 쓰기 어려운 환경의 대안. 현재 OpenAI 문서에서는 beta이며 계정/워크스페이스에서 허용돼야 함. 보안 설정을 임의로 활성화하지 않음 |
| 기존 OAuth 자료 가져오기 | bulk normalizer가 CLI형 중첩 tokens와 평탄한/camelCase export를 정규화. OAuthModal에 bare access token·session JSON을 access-token import API로 보내는 분기 존재 | **새 인증 프로토콜이 아닌 기존 자격증명 재사용.** 이번 경계에서 파일 자동 탐색·업로드·토큰 붙여넣기·복사 요청은 채택하지 않음. import endpoint의 전체 검증·저장 동작은 아래 차단으로 불완전 분석 |
| refresh_token 갱신 | `refreshCodexToken`: refresh_token grant, 새 access/refresh 값 반환. 재사용/만료/invalid_grant/401을 복구 불가로 분류. 다른 실패는 상위 계층에 반환 | 독립 로그인 방식이 아니라 수명 관리. 현 PoC는 refresh·인증 쓰기 금지. 미래 구현에는 회전 토큰 동시 갱신·저장 원자성·실패 처리 검토 필요 |
| `codex-app-server` | 별도 provider. OmniRoute에 OpenAI 토큰을 넣는 대신 로컬 Codex CLI의 JSON-RPC/WebSocket을 사용하고 CLI가 로그인/갱신을 소유. adapter→app-server 연결용 설정은 별개 | 인증 소유를 CLI에 둘 수 있다는 장점은 있음. 그러나 기본 도구 비실행 보장과 실제 도구 결과 전달 계약이 현재 Clauduct 수용 기준을 충족하지 않으므로 현 단계 전환하지 않음 |
| `chatgpt-web-codex` | 별도 web-cookie provider. catalog에 브라우저 cookie와 격리된 headless browser 검증을 사용하는 것으로 명시 | core `codex` OAuth와 구별한다. 브라우저 세션 복사·새 의존성·다른 통신 경로가 필요하므로 채택하지 않음. catalog 표시를 실제 작동 증거로 보지 않음 |
| OpenAI API key | 공식 Codex는 ChatGPT 구독 로그인과 API key 사용량 과금 로그인을 구분 | ChatGPT 구독으로 접근한다는 목적의 대체 수단이 아님. `codex` OAuth normalizer가 API key 로그인도 구현한다는 증거로 해석하지 않음 |

OpenAI가 현재 안내하는 개인 로그인은 ChatGPT와 API key이며 과금/권한 경로가 다르다. device code는 브라우저 callback을 쓰기 어려울 때의 beta 대안이다. 이 공식 안내가 OmniRoute나 Clauduct의 직접 backend 호출을 공식 지원한다고 명시한 것은 아니다. [OpenAI 인증 문서](https://learn.chatgpt.com/docs/auth)

## 3. 재사용할 부분과 그대로 복사하지 않을 부분

재사용할 것은 역할이 분명한 작은 흐름이다: PKCE challenge/verifier와 state 대응, 사용자 브라우저 승인, 고정 token endpoint, 계정/workspace 결합, 제한된 메모리 내 토큰 사용, 만료/401 시 명시적 종료. 현재 core `codex` 요청은 공통 executor에서 `Authorization: Bearer`를 만들고 Codex executor에서 저장된 workspaceId를 `chatgpt-account-id`로 붙인다. 이 계약은 Clauduct의 기존 사용자 검사 방식과 같은 계열이다. 다른 인증방식만으로 새로운 모델 전송 프로토콜을 얻는 것은 아니다.

그대로 복사하면 안 되는 차이는 다음과 같다.

- OmniRoute는 연결 저장·계정 upsert와 조건부 cloud sync 경로를 가진다. Clauduct는 한 계정·한 세션·외부 저장 없는 범위이므로 이 경로를 가져올 필요가 없다. cloud sync가 이 PC에서 활성화돼 있는지는 확인하지 않았다.
- provider/import normalizer의 JWT payload decode는 계정 메타데이터 추출이며 서명 검증이 아니다. 조직 목록에 따른 workspace 선택 휴리스틱이나 기본 만료일 추정으로 Clauduct의 권한/계정을 확정하면 안 된다. 명시적인 계정 결합과 신뢰할 수 있는 수락 증거가 필요하다.
- device flow helper는 polling deadline이 있지만 개별 fetch의 전체 timeout을 독립 보장하지 않는다. 네트워크 예외 후 polling을 계속하는 동작도 있다. 추론 요청 1회 정책과 로그인 polling은 별도 예산으로 설계해야 한다.
- refresh helper 일부 실패 경로는 응답 오류 원문 또는 임의 예외 메시지를 log에 넘긴다. connect 완료 route도 예외 객체를 console.error에 넘긴다. Clauduct에는 고정 분류·허용 숫자/boolean만 남기는 오류 경계가 필요하다.
- 같은 refresh 자료를 여러 도구가 각자 갱신하게 하는 설계는 회전/동시성 문제를 검토해야 한다. 소스 주석의 token family·scope 관련 설명만으로 이 계정에서 실제 충돌을 입증한 것으로 기록하지 않는다.
- 공개 OAuth client_id가 코드에 있다는 사실만으로 Clauduct가 해당 클라이언트 신원·redirect URI·backend 접근을 사용할 자격이나 공식 지원을 얻는 것은 아니다. 이 사용 가능성과 provider 제약은 미확정이다.

코드를 복사하지 않고 구조만 참고했다. 로컬 LICENSE는 MIT다. 향후 코드 자체를 가져올 경우 라이선스 고지를 보존해야 하며, 소스 라이선스와 OpenAI 서비스/인증의 허용 범위는 별개다.

## 4. app-server를 지금 해결책으로 바꾸지 않는 이유

OmniRoute의 `codex-app-server`는 분명한 대안이다. OpenAI 토큰 저장을 라우터에서 제거할 수 있다. catalog의 `noAuth=true`는 “OpenAI 사용에 인증 불필요”가 아니라 **Codex CLI가 인증을 소유한다**는 뜻이다. 로컬 app-server 연결 주소·연결용 인증과 OpenAI OAuth를 구별해야 한다.

읽은 executor는 thread/start에 experimental dynamicTools를 등록하고 sandbox/approval policy를 전달한다. 다만 dynamicTools는 기본 도구 전체 제거의 보증이 아니다. 기존 Clauduct 기록에서도 제한 옵션 뒤 기본 exec/collaboration 선언이 남아, Claude만 도구를 실행한다는 경계를 닫지 못했다. 해당 과거 검사는 이번에 재실행하지 않았다.

또한 OmniRoute는 `item/tool/call`을 downstream에 전달한 뒤, 실제 도구 결과가 아직 없는데 app-server에는 고정 설명과 `success:false`를 응답하고 현재 turn을 끝낸다. 도구 결과는 다음 전체 이력 요청으로 보낸다. 이는 Clauduct가 요구하는 도구 결과·상태 보존과 같은 계약이라고 가정할 수 없다. 수용하려면 별도 근거가 필요하다. sandbox/approval 설정을 바꾸거나 guard를 우회해 이 대안을 실행하지 않는다.

## 5. 근거 파일과 검증 경계

모든 경로는 위에 고정한 clone commit 기준이다.

| 근거 | 확인한 범위 |
|---|---|
| [Codex OAuth provider](../../_ref/OmniRoute/src/lib/oauth/providers/codex.ts) | PKCE/port/callback, code 교환, 토큰·workspace 매핑 |
| [Device flow](../../_ref/OmniRoute/src/lib/oauth/codexDeviceFlow.ts) | usercode/poll/token 교환, timeout·취소·오류 처리 |
| [사용자 device connect 페이지](../../_ref/OmniRoute/src/app/connect/codex/[token]/CodexConnectClient.tsx), [완료 route](../../_ref/OmniRoute/src/app/api/codex/connect/[token]/route.ts) | 사용자 브라우저 → 일회 ticket 검증 → 연결 저장 호출 |
| [OAuth 공통 route](../../_ref/OmniRoute/src/app/api/oauth/[provider]/[action]/route.ts) | codeVerifier 요구, exchange, 연결 upsert, 조건부 cloud sync 호출. 전체 auth 감사는 아님 |
| [OAuthModal](../../_ref/OmniRoute/src/shared/components/OAuthModal.tsx), [bulk normalizer](../../_ref/OmniRoute/src/lib/oauth/services/codexImport.ts) | 가져오기 UI 호출과 순수 변환. 실제 import endpoint의 파일/저장 동작은 미검증 |
| [Codex refresh helper](../../_ref/OmniRoute/open-sse/services/tokenRefresh/providers/codex.ts) | refresh grant, 회전 반환값, 오류 분류·로그 |
| [공통 executor](../../_ref/OmniRoute/open-sse/executors/base.ts), [Codex executor](../../_ref/OmniRoute/open-sse/executors/codex.ts) | Bearer, workspace header 조립 |
| [app-server catalog](../../_ref/OmniRoute/src/shared/constants/providers/noauth.ts), [executor](../../_ref/OmniRoute/open-sse/executors/codex-app-server.ts) | CLI 인증 소유 표시, dynamic tool 반환 처리·thread policy |
| [web-cookie catalog](../../_ref/OmniRoute/src/shared/constants/providers/web-cookie.ts) | 별도 ChatGPT Web (Codex) 인증 표시. 실제 browser 구현/통신은 미검증 |

**Verified:** 위 소스의 정적 호출 관계, 참조 clone 버전·clean 상태, 현재 공식 로그인 방식. Clauduct 오프라인 변환 검사와는 별개 증거다.

**Not verified:** 실제 OmniRoute의 활성 인증방식·설정·저장소, 이 계정에서 새 로그인/device code 사용 가능 여부, PKCE/session 전반의 보안 감사, 인증 endpoint 실제 동작, refresh 동시성·원자적 저장, Clauduct의 OAuth client 신원 사용 가능성, 실제 Claude 도구 왕복.

**Blocked by:** 인증 파일 이름을 포함한 소스 검색 명령 1건이 `credential-path` guard에 차단됐다. guard는 검색어와 실제 인증 경로를 구분할 수 없다는 사유를 반환했다. 해당 명령에 묶였던 OAuth constants·Codex import route/helper·ChatGPT Web constants의 추가 열람은 재시도하지 않았다. 다른 경로로 동일한 차단 효과를 재현하지 않았고 실제 인증 파일에도 접근하지 않았다. 가져오기 세부 판정은 이미 읽은 normalizer/UI 근거로 제한한다.

다음 설계 우선순위는 공개 합성 marker를 쓰는 Claude 요청 호환성 확인, 그 뒤 기존 Codex OAuth의 사용자 운영·메모리 수명 경계다. 단일 계정 binding, 만료/401 중단, 실패 로그 비노출, refresh·쓰기 없는 범위를 먼저 검증한다. 새 PKCE를 선택하는 경우에만 client/redirect와 callback의 state/Host/Origin·일회성·timeout 조건을 추가한다. 현재 guard 거부를 새 OAuth·app-server·cookie 경로로 우회하는 단계는 포함하지 않는다.
