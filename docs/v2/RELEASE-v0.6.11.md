# v0.6.11 — 라우팅 불변식, 경계 프로필, 공개 빌드 Agent View

⟦TBD:ship-date⟧ 출시(태그 `v0.6.11` → `⟦TBD:tag-commit⟧`, [정식 Release](https://github.com/wotjr1649/Clauduct/releases/tag/v0.6.11)). 2026-10-16 초안에서 시작했다. 범위는 v0.6.10 뒤 기본 branch에 들어간 PR(#296–#318)과
[#319](https://github.com/wotjr1649/Clauduct/issues/319)·[#320](https://github.com/wotjr1649/Clauduct/issues/320) 개발 브랜치다.
남은 제한과 다시 볼 조건은 [COMPATIBILITY.md](COMPATIBILITY.md) 3절의 "알려진 제한 원장"과 "v0.6.11 — 지원 환경과 보장 한계" 표가 관리한다.

**2026-10-22 갱신.** 최초 초안과 아래 날짜별 관문 이력은 그대로 보존한다. `cc604134` 이후 변경과 미커밋 작업을
추가했으며, 현재 후보의 최종 관문·실제 backend·재측정·출하 결과는 `TBD(gate)`다. 과거 후보의 PASS를 현재 후보에 승계하지 않는다.

**2026-10-23 갱신.** 아래 2026-10-22의 미커밋·패치 준비·재측정 미완료 표시는 당시 이력으로 보존하고, 현재 상태는
"2026-10-23 추가"와 "native" 절의 갱신으로 대체한다. 측정 기준은 native 2.1.305·Codex CLI 0.163.0이다. 무과금 재측정은
완료했지만 현재 후보의 최종 전체 관문·실제 backend·출하는 여전히 `TBD(gate)`다.

**출하 상태(⟦TBD:ship-date⟧).** 이 문서의 날짜별 `TBD(gate)` 표시는 당시의 미완료 표시로 보존한다. 출하 후보의 최종 결과와
그 표시들의 정산은 "출하 검사"의 "최종 출하 후보" 절이 소유한다. 출하 전에 끝나지 않은 항목은 후속 Issue로 남겼다.

## 바뀐 것

- **모델 요청 라우팅 불변식**([#295](https://github.com/wotjr1649/Clauduct/issues/295), PR #296)
  - settings 파일(user·project·local) env의 endpoint·cloud provider 변수·unix socket이 모델 요청을 옮기던 것을 세션 `--settings`
    env 고정으로 막는다. `NO_PROXY`에 loopback을 더한다.
  - 라우팅을 정하는 기기 정책(managed 출처)이 있으면 시작하지 않는다(`MANAGED_ROUTING_POLICY`). 실행 중에는 hook이 native 환경을
    대조한다.
- **저장된 Claude apps gateway 로그인 감지**([#299](https://github.com/wotjr1649/Clauduct/issues/299), PR #317): native 시작 모델 목록의 순서는 첫 hook보다 앞선다고 보장되지 않는다.
  SessionStart와 매 `UserPromptSubmit` hook이 proof를 보내고 같은 native PID·생성 시각의 직접 `/v1/models` 요청을 최대 5초 기다린다. 이전 generation native도 즉시 거부하지 않으며, 같은 PID·생성 시각의 목록이 기한 안에 없을 때 `ROUTING_PROOF_PRIOR_GENERATION`으로 분류한다. 다른 native의 목록은 proof가 아니다. 같은 native 목록이 오면 204를 돌려보내고, 기한까지 오지 않으면 409다. 무제어 경로는 `ROUTING_UNVERIFIED` fail-closed mismatch로 세션을 끝내고, 인증된 provider-control 경로는 prompt마다 최대 5초 기다린 뒤 일반 proof 누락을 진단으로만 기록한다. 단, resident gateway 시작 전부터 실행된 native의 같은 PID·생성 시각 목록이 끝내 없으면 controlled 여부와 무관하게 409 `ROUTING_PROOF_PRIOR_GENERATION`으로 거부하고 hook이 event를 막는다. UserPromptSubmit hook은 해당 user prompt의 모델 요청을 통과시키기 전에 proof 결과를 확인한다.
  증명 요청의 HTTP 예산은 7초이며, 다른 hook HTTP 요청은 3초다. 추가 대기로 저장된 로그인 판정과 무제어 경로의 차단이 이전보다 최대 3초 늦어질 수 있다. 그동안 모델 목록 외에도 어떤 종류든 prompt 전 native 요청이 저장된 gateway에 도달해 처리될 수 있다. hook이 실행되지 않는 환경과 늦은 모델 목록은 COMPATIBILITY 원장의 해당 행에 있는 제한이다.
- **Anthropic 경계 프로필**([#301](https://github.com/wotjr1649/Clauduct/issues/301), PR #306)과 **서버 경계 원장**
  ([#305](https://github.com/wotjr1649/Clauduct/issues/305), PR #308): `boundary_profile`의 `default`·`strict`.
  [SETTINGS](SETTINGS.md#경계-프로필-v0611)와 COMPATIBILITY 3절 "서버 경계 원장"에 있다.
- **native 기능 표면 대조**([#297](https://github.com/wotjr1649/Clauduct/issues/297) PR #298,
  [#300](https://github.com/wotjr1649/Clauduct/issues/300) PR #303): 재측정이 표면 관문을 돈다. 세션은 측정 기준에 없는 tool과
  받지 못한 기본 tool을 status와 종료 줄(`added_tools=`·`missing_tools=`)에 보인다. 보고이고 차단하지 않는다.
- **`max_tokens`를 backend 생성 한도로**([#309](https://github.com/wotjr1649/Clauduct/issues/309), PR #316): 생성 요청에
  `max_output_tokens`로 보낸다. thinking을 끈 요청은 보내지 않는다. 완료 뒤 검사는 유지한다.
- **역할 없이 부른 teammate 시작**([#307](https://github.com/wotjr1649/Clauduct/issues/307), PR #311)
- **종료 때 취소된 native 보조 생성을 따로 셈**([#310](https://github.com/wotjr1649/Clauduct/issues/310), PR #318):
  `auxiliary_cancelled`. 상태 JSON 전체가 출력되던 것을 고쳤다.
- **MEASURE 항목 정리**([#312](https://github.com/wotjr1649/Clauduct/issues/312), PR #313), **발행 뒤 문서 대조와 출하 때 backend
  capability 재측정**([#314](https://github.com/wotjr1649/Clauduct/issues/314), PR #315, [PACKAGING](PACKAGING.md) 4.1)
- **PDF 렌더러가 stdin 입력을 받음**([#322](https://github.com/wotjr1649/Clauduct/issues/322)): native 2.1.292가 입력을 `-`로
  주고 문서를 stdin에 쓰면 거부하던 것을 같은 크기 상한으로 받는다.
- **지원 환경과 보장 한계**(#319·#320, 결정 17): provider-control admission을 launch 필수 조건에서 빼고 진단 전용으로 둔다.
  진단 도구 `clauduct-probe.exe`를 `clauduct.exe`와 한 쌍으로 싣는다([PACKAGING](PACKAGING.md) 1절). 지원·제외 환경과 남는 위험은
  COMPATIBILITY 3절 "v0.6.11 — 지원 환경과 보장 한계" 표에 있다.
- **background와 연결 유지**(#319)
  - 소유 작업의 native stop 명령은 공통 30초 예산에서 실행되며, 한 작업의 Wait를 기다리지 않고 다른 소유 작업에도 stop을 발송한다. 다만 공통 예산이 만료되면 일부 작업에는 발송하지 못할 수 있다. 취소 중 stop 실패는 기존 정리 진단에 남긴다. native 명령의 성공(ACK), `done`·`failed`·`stopped` 상태, worker의 실제 OS 종료는 서로 다르다. 후속 상태가 없거나 live로 남아 있어도 stop 명령의 exit 0은 ACK 수준 성공이며 worker·자손의 완전 종료를 보장하지 않는다. native 자체의 종료/kill 동작도 모든 경로에서 특정 시간 안에 실행·성공함을 보장하지 않는다. 기존 moved-link 경합은 남는다.
  - Claude 계정 명령(`auth login`·`auth logout`·`setup-token`)은 시작 전에 거부하고 계정 slash command를 숨긴다.
  - `--resume <session id> --bg`는 native가 copy로 시작하므로 `--fork-session` 없이 쓰면 거부한다(`BACKGROUND_RESUME_IS_A_COPY`).
  - 연결 유지 프로세스가 사라진 job은 resident record로 같은 port·link에 다시 띄운다. 복원은 창 없는 relay를 거치고, 시작 실패는
    `CLAUDUCT_RESIDENT_RESTORE_FAILED: <category>`로 알린다. 재생성한 프로세스는 이전 세대가 증명한 살아 있는 native의 routing
    증명을 이어받는다. 해당 native의 이전 증명을 복구하지 못하면 `ROUTING_PROOF_PRIOR_GENERATION`으로 prompt를 거부하고 `claude respawn <job>`을 안내한다.
  - `clauduct rm`·`stop`·`logs`는 세션을 열지 않는 관리 경로다. 삭제된 job의 record와 events 디렉터리를 회수한다. job이 모두
    done·stopped이고 등록된 native가 하나 이상이며 모두 종료됐고 처리 중인 요청도 없는 상태가 3초 이어지면 연결 유지
    프로세스가 세션을 놓는다.
  - 최종 상태를 확인할 수 없는 기록은 `FINAL_STATUS_UNVERIFIED`로 보인다. Codex 미로그인은 `CREDENTIAL_UNAVAILABLE_OR_EXPIRED`와
    `codex login` 안내로 끝난다.
- **native 프로세스와 세션 신원**(#319)
  - 요청마다 보낸 native 프로세스를 OS(연결 소유자, PID와 생성 시각)로 확인하고, native 프로세스마다 현재 세션을 따로 둔다.
    확인 질문의 답은 그 요청을 보낸 native의 것만 받는다.
  - fork는 증명된 부모에서만 model·effort·도구 이력을 이어받는다. 부모는 launch의 resume 대상, 그 native가 등록한 세션, 새
    native 프로세스의 `--resume <원본> --fork-session` 실행 인자, 유일한 live 후보에서 온다. 대화 fork의 부모 메시지는 그 세션의
    활성 chain에 있는 최상위 메시지여야 한다. 증명하지 못하면 `SESSION_PROFILE_UNVERIFIED`로 거부하고 부모 세션은 계속 쓸 수 있다.
  - native의 session recap(`/recap`, attach 때 요약)을 도구 호출 없는 추론으로 처리한다. 전에는 확인 절차가 거부했다.
- **effort**(#319): 시작 effort를 세션 `--settings`로 넘겨 무입력 dispatch에도 쓴다. `modelDefaults`의 luna 공장값은 `xhigh`다
  ([SETTINGS](SETTINGS.md)).
- **공개 빌드 foreground Agent View**(#319)
  - foreground가 `CLAUDE_CODE_DISABLE_AGENT_VIEW`를 더 이상 넣지 않는다. TUI의 `/background`·`/bg`와 Agent View를 쓸 수 있다.
  - Clauduct 값(gateway 주소, 세션 기본값, 필수 환경)은 `--settings` env로, token은 session link의 key helper로 넘긴다. native의
    OS 환경에는 넣지 않는다. 전에는 Clauduct가 먼저 띄운 native supervisor 아래에서 사용자의 `claude --bg` worker 환경에 빈
    credential 2개와 Clauduct 정책 값이 섞였다(합성 측정. 주소·token·모델 값은 없어 요청이 옮겨 가지는 않았다).
  - 세션 기본값은 OS 값이었을 때처럼 settings 파일과 셸의 같은 이름에 양보한다. 필수 값과 routing 값은 settings 파일이 쓴 모든
    철자와 사용자 `--settings` 병합 뒤에도 고정된다. 셸의 `ANTHROPIC_*`는 native에 가지 않는다.
  - hook은 환경 이름을 대소문자 없이 읽어 모든 철자를 대조하고, 값이 다른 중복 철자는 거부한다. native는 settings 파일이 쓴
    마지막 철자를 hook에 넘기므로, 파일에 소문자 `anthropic_base_url`만 있어도 routing 증명은 session link의 주소로 간다.
  - native가 실행 중일 때 foreground는 key helper가 자기 session link를 정확히 가리키는 job을 100 ms마다 찾아, 처음 본 때
    resident record를 쓴다. native가 정상 종료하면 record를 쓴 뒤 터미널을 돌려주고, launcher의 pipe가 없어도 콘솔에서 떨어져 job을
    계속 서비스한다. 마감·취소·`-p` 인터럽트·라우팅 중지에서는 인계하지 않고 같은 link의 job을 끝낸다. 3초 job 검색은 인계가
    가능한 실행에서만 한다.
  - foreground는 현재 key helper가 자기 link를 가리키는 job만 record·handover 대상으로 삼는다. 같은 session을 foreground로
    resume한 실행이 그 session의 `--bg` job을 가져가 native `agents`의 wake가 `SESSION_ALREADY_IN_USE`로 실패하던 개발 중 결함을
    고쳤다.
  - resident record는 primary job의 session을 기록한다. 연결 유지 프로세스는 유지할 job의 프로필이 모두 등록된 뒤, job이 소유하지 않는
    원래 foreground 세션의 등록 native가 모두 종료되고 사용 중인 요청이 없으면 프로필을 저장하고 잠금을 해제한다. job 상태나
    native 종료를 확인하지 못하거나 저장에 실패하면 잠금을 유지한다.
- **print 취소 상태**(#319): Ctrl+C로 끝난 native의 종료를 콘솔 신호보다 먼저 관측해도 `USER_CANCELLED`로 기록한다.
  `STATUS_CONTROL_C_EXIT`가 아닌 종료 코드와 interactive 실행의 처리는 그대로다.
- **분류기 요청 진단**(#319): auto 모드 분류기로 판정된 요청의 상태 기록에 `classifierStage`(1·2)와 `classifierVerdict`
  (block·allow·unparsed·none)를 남긴다. native 2.1.300이 읽는 방식으로 판정을 읽는다. 답 본문은 남기지 않고 routing과 native가
  받는 답은 바뀌지 않는다.
- **backend 표시 metadata**(#319): 알려진 `response.metadata`의 `openai_chatgpt_moderation_metadata` 객체를
  표시 알림으로 처리한다. 내용을 전달하거나 저장하지 않고 `moderationMetadata` 횟수만 센다. 모르는 필드와 잘못된 형태는
  계속 거부하며, routing·완료·`safety_buffering` 검증은 유지한다.
- **확인 응답 시간 초과**(#319): 확인 왕복 2초 안에 그 native의 helper가 아무 답도 보내지 못한 요청은 그 요청만
  `NATIVE_CONFIRMATION_TIMEOUT`(HTTP 503)으로 거부한다. 전에는 그 native를 수명 내내 거부 상태로 묶어 한 번의 지연이 세션을
  끝냈다. native가 같은 step에서 재시도하고 재시도도 새 확인값을 받아야 한다. 규약을 어긴 답, 형식이 틀린 답, 읽을 수 없는 취소
  영수증, 저장소 손상은 계속 거부 상태로 묶는다. native가 helper를 시작하지 못한 경우(2.1.303 Windows: 출력 전 종료 코드 1)도
  그 요청만 거부한다([SETTINGS](SETTINGS.md)).
- **저장된 native gateway 시작 전 거부**(#319): 새 세션은 같은 native의 `auth status --json`으로 저장 provider 상태를 먼저 본다.
  저장된 Claude apps gateway면 `NATIVE_STORED_GATEWAY`, 확인하지 못하면 `NATIVE_PROVIDER_STATUS_UNVERIFIED`로 모델 세션 전에
  거부한다. 실행 중 바뀌는 credential은 인증하지 않는다([NATIVE-SUPPORT](NATIVE-SUPPORT.md)). 시작 전 native provider 상태 확인에는 30초 한도를 적용하며 호출자의 마감이 더 이르면 먼저 종료한다. 확인과 만료 후 최대 5초의 프로세스 종료·수거 대기 때문에 **새 진행 출력 없이 약 35초까지 기다릴 수 있다**. 동기 OS 호출이나 스케줄링 지연이 있으면 전체 경과 시간은 더 길어질 수 있다. provider 상태 검사에서 거부하면 모델 세션을 시작하지 않고 stderr에 내용 없는 메타데이터 다섯 항목, 실패 단계(`stage`), 고정 사유(`reason`), 정리를 포함한 경과 밀리초(`elapsed_ms`), 수거 여부(`reaped`), 종료 코드 또는 unknown(`exit`)을 한 줄로 남긴다. credential과 query 출력 내용은 기록하지 않는다.
- **hook 시간 예산**(#319): 사용자·plugin hook의 timeout과 생략된 설정은 그대로 두고 native가 집행한다. Clauduct가 만든 bridge
  hook에만 외부 30초를 명시한다. 제품 hook은 공유 18초 안에서 끝나며, proof 요청은 최대 7초를 쓴다([NATIVE-SUPPORT](NATIVE-SUPPORT.md)).
- **생성 worker 역할의 도구**(#319): Clauduct가 생성하는 worker 역할이 부모의 native 도구 집합을 상속한다. 고정 목록 때문에
  WebFetch가 막히던 것을 고쳤다. 사용자 `--agents`의 명시적 제한은 유지한다.
- native가 unset으로 읽는 `CLAUDE_CODE_RETRY_WATCHDOG=0`·`CLAUDE_CODE_RESUME_INTERRUPTED_TURN=0`을 넣지 않는다. Go는 1.27.2다.

### 2026-10-22 추가 (`cc604134` 이후)

- **버전 고정 없는 native 신원 확인**(`563fda5d` 및 미커밋 보완)
  - native 버전을 허용목록에 고정하지 않고 실제 실행 이미지와 파일 신원을 대조한다. 짧게 실행되는 native도 확인 대상으로 삼으며,
    자동 업데이트를 전역에서 끄지 않는다. 이미지 교체나 확인 실패는 같은 실행 파일이라는 증명으로 취급하지 않는다.
  - 미커밋 보완은 짧은 프로세스의 파일 신원 연결과 확인 실패의 구분을 다룬다. 최종 반영·자동 갱신 중 동작·실제 검증은
    `TBD(gate)`다. 아래 과거 관문의 읽기 잠금 기록은 당시 방식이며 새 후보의 검증 방식으로 승계하지 않는다.
- **PR #330 리뷰에 따른 수정**([PR #330](https://github.com/wotjr1649/Clauduct/pull/330), `abc6e56b`·`e6eabfd4`·`7a12005e`)
  - 복원한 형제 세션은 자신의 저장 model·effort와 대조한다. 거부된 재개는 프로필을 저장하지 않고, 그 실패가 다른 native의 같은
    세션 요청을 막지 않도록 했다. 큰 이미지·PDF·도구 결과가 든 대화 행도 부모 메시지 확인에서 잘리지 않게 했다.
  - PID 재사용으로 다른 세션을 종료하지 않도록 부모 생성 시각을 확인한다. 읽을 수 없거나 손상된 turn 기록은 없는 기록으로
    취급하지 않는다. 종료가 확인된 native 등록을 회수해 반복 respawn이 과거 등록 수 때문에 막히지 않게 했다.
  - 경로는 한 번만 디코딩하고, 이미지 신원의 큰 정수는 정밀도를 잃지 않게 기록한다. 업데이트 실패 뒤 원상복구까지 실패하면
    모두 복구했다고 알리지 않고 남은 파일을 보고한다. 이 수정들을 포함한 최종 관문·실제 backend·업데이트 검증은 `TBD(gate)`다.
- **경로 template과 소유자 결정**(`f29d8e9e`·`0470c919`)
  - 요청 경로는 동적 값을 가린 template으로 기록한다. 2.1.304까지의 추출에 2.1.305 bundle을 더해 283개에서 369개로 늘었으며,
    host 목록은 유지됐다. 추출은 native 호환성 PASS나 통신 허용 판정이 아니다.
  - 2026-10-22 결정은 추가 후보 86개 중 **78개 기록용 채택·4개 제거·4개 보류**다. 제거 대상은 `/api/eval`,
    `/worker/diagnostics`, `/worker/register`, `/worker/{seg}`다. 보류 대상은 `/api/frame/comments/{seg}/{seg}/{seg}`,
    `/api/oauth/local_pairing`, `/api/oauth/organizations/{seg}/sync/github/auth`, `downloads.claude.ai`의 `/model-catalog/v1/schema.json`이다.
  - 보류 4개는 표에 남기되 재실행 전 HOLD로 표시한다. 제거 후 표는 365개가 되며 Go 표와 공유 grammar를 함께 반영해야 한다.
    결정에 따른 패치는 준비 단계다. 최종 반영·재실행은 `TBD(gate)`이며, **기록용 채택은 트래픽 승인이 아니다.**
- **관측기 고부하 대응**([#325](https://github.com/wotjr1649/Clauduct/issues/325))
  - 다른 프로그램의 통신이 많을 때 대기열 포화로 진단 처리가 지연되는 상황을 줄이고 종료 실패 진단을 보존하는 패치를 준비했다.
    관측 누락·overflow를 성공으로 넘기지 않는 판정은 유지한다. 최종 반영·실제 고부하·종료 검증은 `TBD(gate)`다.
    아래 r5 실패 기록과 단독 재실행 결과는 그대로 유효한 과거 이력이다.
- Go 1.27.2 고정을 모듈 선언에도 반영했다(`262d7904`). 최종 출하 빌드와 재현성 확인은 `TBD(gate)`다.

### 2026-10-23 추가 (`98990cbd` 초안의 후속 갱신)

- **native 신원 capture의 대기 판정**(`5d36c9b0`): `WAIT_OBJECT_0`만 종료, `WAIT_TIMEOUT`만 생존으로 판단한다.
  그 밖의 결과는 모두 확인 불가로 남겨 생존이나 종료의 증명으로 쓰지 않는다. 빠른 capture에는 DOS 경로와 함께 실제 매핑의
  NT 이름을 기록한다. 이 변경을 포함한 최종 관문·실제 backend 결과는 `TBD(gate)`다.
- **경로 template 결정 반영**(`028cd1ce`): 위에 나열한 4개를 제거해 표는 365개가 됐고, 보류 4개는 재실행 전까지 HOLD로
  남겼다. 위 "패치는 준비 단계"와 "최종 반영" 미완료 표시는 이 반영 사실로 대체한다. 재실행 결과는 `TBD(gate)`이며,
  **기록용 채택은 트래픽 승인이 아니다.**
- **측정 기준과 probe digest 갱신**(`c6264df9`): 2026-10-23 무과금 재측정으로 native 2.1.305를 반영했다.
  Codex CLI는 0.163.0으로 유지했다. 도움말·plugin API·도구 표면은 버전 줄 외에 차이가 없고, fixture 묶음과 표면 관문은
  PASS였다. `clauduct-probe.exe` digest도 이 소스 트리에 맞춰 갱신했고 `-trimpath -buildvcs=false` 빌드의 재현성을 확인했다.
  이 digest는 출하 자산의 SHA-256을 대신하지 않으며, 최종 태그의 빌드·digest는 `TBD(gate)`다.
- **신원·송신자 귀속 검증 보강**
  - 버전 고정 없는 native 신원 검증은 모든 증거 형식을 하나의 엄격한 관문으로 판정한다. 프로세스 세대는 PID와 정확한 생성
    시각의 쌍이며, 신원 확인과 네트워크 귀속에 시간 허용 구간을 쓰지 않는다.
  - 살아 있는 연결 소유자의 handle 생성 시각을 얻지 못하면 CIM의 마이크로초 해상도에서 정확히 일치하는 유일한 세대에만
    연결한다. 같은 PID나 가까운 시각만으로 소유자를 인정하지 않는다.
  - 관측기 고부하 내성([#325](https://github.com/wotjr1649/Clauduct/issues/325))과 종료 실패 진단도 보강했다.
    관측 누락·overflow는 계속 성공으로 인정하지 않는다. 보강 후 최종 통합·실제 고부하·종료 검증은 `TBD(gate)`이며,
    아래 r5 실패와 단독 재실행 이력은 그대로 보존한다.

### 2026-10-24~28 추가 (`68f4373f` 이후)

- **측정 기준 native 2.1.309~2.1.312**(`2f506afe`·`506520b6`·`e7a7be1e`·`4b963405`): 자동 갱신된 native를 버전마다 무과금
  재측정해 측정 기준과 `clauduct-probe.exe` digest를 갱신했다. Codex CLI는 2.1.309부터 0.164.0이다. 결과는 "native" 절에 있다.
- **경로 template 5개 추가(365 → 370)**: `/worker/sender_addr`(2.1.309), `/worker/agent-metadata-events/batch-get`(2.1.310),
  `/worker/event-batch`와 `platform.claude.com`의 `/docs/mcp`(2.1.311), `/api/organizations/{uuid}/dust/generate_title_and_branch`(2.1.312).
  host 목록은 그대로다. 문자열 표의 추출 잡음은 넣지 않았다. **기록용 채택은 트래픽 승인이 아니다.**
- **기록만 하고 경로표에 넣지 않은 관찰**(2.1.309 정적 판독)
  - Google Cloud provider(`claude.googleapis.com`)의 base 경로가 `/v1/projects/{p}/locations/{l}`로 바뀌었다. launch가 provider
    전환 변수를 지우므로 Clauduct가 띄운 native에서는 쓰이지 않아야 한다. 보류했다([#337](https://github.com/wotjr1649/Clauduct/issues/337)).
  - CCR 세션은 `/worker/sender_addr` 응답으로 받은 두 값을 이후 요청의 `x-ssr-client-ip`·`x-ssr-client-ip-sig` 헤더로
    붙인다(정적 판독). 경로 template은 헤더를 기록하지 않으며 실제 송신은 측정하지 않았다([#338](https://github.com/wotjr1649/Clauduct/issues/338)).
- **routing proof 대기 5초**(`f38cf2b6`): 위 "저장된 Claude apps gateway 로그인 감지" 항목에 반영했다. gateway가 닫히는 중이면
  proof에 503 `GATEWAY_CLOSED`로 답하고, hook은 이를 종료로 처리해 mismatch로 보고하지 않고 prompt를 막는다.
- **PR #330 리뷰 수정 2차**(`4aa8b3a7`): Agent View 인계 조건과 link 한정 중지(위 "공개 빌드 foreground Agent View"),
  `state.json` 없는 job 디렉터리를 부재로 보지 않음, PDF alias PATH 완화(native settings 파일이 PATH를 정의하거나 읽을 수 없으면
  쓰지 않음).
- **다른 native의 재개 실패 격리(B10)**(`fb66791b`): 다른 native에서 재시작이 필요한 재개가 실패해도 신원이 확인된 정상
  소유자의 같은 세션 요청과 준비 상태를 막지 않는다. 재시작이 필요한 native의 요청은 계속 거부하며, native 신원이 없는 요청은
  같은 세션의 재시작 대기를 구분할 수 없어 계속 거부한다.
- **검증의 native 신원 판정 정리**: 출하 관문의 판정기는 실행 이미지를 확인하지 못한 증거(이미지 분류 `unknown`, capture 오류나
  누락)를 부재나 성공으로 보지 않는다. 그런 프로세스는 같은 세대의 승인된 연결이나 유효한 capture가 있을 때만 받고, 없으면
  그 행을 PASS로 두지 않는다. 제품 실행 파일의 동작은 바뀌지 않는다.

### 2026-10-30 추가 (`5926b619` 이후)

- **측정 기준 native 2.1.314**(`cb069eab`, probe digest 갱신 commit `123c7632`): 2026-10-30 무과금 재측정에서 도구 표면은 버전 줄 외에
  같았다. 도움말은 `auto-mode` 설명 문구(`classifier` 삭제)만, plugin API는 `RenderComponent`의 `Placed`와 `$.ui.place` 선언
  추가만 달랐고 Clauduct는 두 선언을 쓰지 않는다. fixture 시험과 표면 관문이 통과했다. 설치본 Codex CLI는 0.166.0으로 자동
  갱신됐지만 도움말이 같아 측정 기준은 0.164.0으로 두었다.
- **경로 template 9개 추가(370 → 379)**: 2.1.314 정적 판독에서 본 `/v1/code/channels`(`/{seg}`, `/{seg}/config`,
  `/{seg}/messages`, `/{seg}/setup-proposals` 포함), `/v1/code/mcp/meta`, `/v1/code/memory/channel/{seg}/memories`(`/{seg}` 포함),
  `/v1/filestore/fs/readFileMetadata`. 모두 host를 가리지 않는 기록 template이며 host 목록과 제외·HOLD는 그대로다.
  **기록용 채택은 트래픽 승인이 아니다.** 제품 라우팅은 바뀌지 않았다.
- **받은 upstream 전송 오류 보존**(`0248efbc`): upstream 응답 chunk와 함께 EOF가 아닌 읽기 오류가 왔는데 같은 chunk의
  downstream 전달도 실패하면(클라이언트 취소나 소켓 쓰기 오류), 전달 실패(`CANCELLED`·`DELIVERY_FAILED`)가 받은 오류를
  덮지 않고 응답 해석의 판정(`TRUNCATED_STREAM`·`INVALID_UTF8`)으로 끝낸다. 비스트리밍 요청은 전달 실패가 취소일 때만 이
  규칙을 쓴다. 시각에 따라 갈리던 시험 실패에서 찾았고, 결정적으로 재현해 실제 HTTP 회귀 시험으로 고정했다. 대상은 스트리밍
  relay에서 read 직후 owner context가 살아 있는 경우다. 이 분기는 실제 backend PASS 없음(offline real-HTTP 회귀만 있음)이다.
  실제 backend 연결은 HTTP/2라 데이터와 오류가 보통 따로 반환되고, 같은 chunk의 client 쓰기 실패 시점도 제어할 수 없다.
  그래서 출하 판정에서는 이 분기를 문서화된 한계로 둔다(소유자 결정 2026-10-31). fault injection 검사는
  [#342](https://github.com/wotjr1649/Clauduct/issues/342)가 이어받는다.
- **ctl v2 검사용 ptydrive 단계 관찰**(`cdb5aa8b`): 검증용 ConPTY driver가 입력·대기 pattern·화면 내용 없이 고정된 driver
  상태만 알리는 단계 관찰자를 받는다. ctl v2 control 행이 CR 쓰기 시점을 감싸고 driver 신원을 결속하는 데 쓴다. 관찰자 없는
  기존 사용은 그대로이며 제품 실행 파일의 동작은 바뀌지 않는다.

### 2026-10-31~11-01 추가 (`123c7632` 이후)

- **provider-control 진단 probe 보강**(`e57723e7`·`9d878bbb`·`a31f015d`, probe digest 갱신 commit `713b4a65`·`e116601e`·
  `be3ce5cd`): `clauduct --dev --probe provider-control`의 case를 차례로 실행하고, 수신 연결 기록과 child 종료 증거를 보강했다.
  진단 전용이며 일반 launch는 admission을 쓰지 않는다. gateway 요청 기록에 진단 필드(`nativeRouteScope`·`confirmationSHA256`·
  `responseDelivered`)가 늘었고 응답 내용과 라우팅은 바뀌지 않는다. probe의 전체 8개 entry ADMITTED 증거와 CTL v2 controller
  campaign은 이번 출하에 포함하지 않았다. R04L은 같은 후보의 MULTI_TURN 무과금 진단 3회에서 INCONCLUSIVE였다
  ([#340](https://github.com/wotjr1649/Clauduct/issues/340), "최종 출하 후보").
- **연결이 없는 `--background-stop`의 오류 문구**(`9d878bbb`): 열린 pipe 인스턴스가 없는 연결 ID에 `clauduct --background-stop <연결 ID>`를
  실행하면 표준 오류에 `BACKGROUND_CONNECTION_UNAVAILABLE: pipe not found`를 쓰고 종료 코드 1로 끝난다. 이전에는
  `BACKGROUND_CONNECTION_UNAVAILABLE`만 썼고 종료 코드는 같다. 그 밖의 연결 실패 문구는 그대로다. 이 변경의 실제 backend 검증은
  "최종 출하 후보"의 "실제 backend" 결과에 포함한다.
- **측정 기준 native 2.1.316**(`cc8393b1`, probe digest 갱신 commit `6ddb44d8`): 2026-10-31 무과금 재측정에서 도움말·plugin API는
  같았고 도구 표면은 버전 줄 외에 같았다. fixture 시험과 표면 관문이 통과했다. 2.1.314와 비교한 정적 추출의 route·host 집합도
  같아 경로표는 그대로다(379개). 설치본 Codex CLI는 0.166.0이지만 도움말이 같아 측정 기준은 0.164.0으로 두었다. 다만
  native가 보내는 system prompt는 2.1.314와 비교해 6개 경로 모두 달랐다. 버전 헤더가 바뀌었고 코드 스타일 지시 한 문장
  ("Write code that reads like the surrounding code: …")이 빠졌다. 소유자는 이를 native 자체의 변경으로 받아들였다(2026-11-01).
  Clauduct 구현은 변경하지 않았다. 달라진 prompt는 backend 입력에 전달되며 응답에 미치는 영향은 이 비교로 확인하지 않았다.

### 2026-11-03 native 재측정

- **측정 기준 native 2.1.317**(SHA-256 `26b01ec2…6038`): 무과금 재측정과 fresh print/TUI × reference/clauduct/strict 표면 관문이 통과했다.
  2.1.316과 도움말·도구 이름·core 집합은 같고, plugin API에 desktop용 `Backdrop`·`$.ui.backdrop`가 추가됐다. 제품 plugin은 이 API를 사용하지 않는다.
  print 목록 세 실행 방식에는 내장 `cc-plugin-privacy`가 추가됐다. 시스템 prompt의 실행별 경로·UUID를 정규화하면 버전 헤더만 달랐다.
  정적 추출에는 `/api/frame/artifacts`, `/v1/organizations/analytics/cost`와 `gmailmcp.googleapis.com`이 추가됐다. 첫 경로는 기존 기록 pattern에
  포함되고, 비용 집계 경로는 bounded generic fallback으로 남긴다. 정적 문자열을 통신 승인으로 확대하지 않아 Go 경로표(379개)·host 표·grammar·approval은 유지한다.
  정적 판독에서 native 내부 loopback MCP 인증이 URL 경로의 secret에서 Bearer header로 이동했다. 이 내부 구현 변경을 모든 plugin/MCP 조합의 실행 성공으로 확대하지 않는다.
  이번 평가에서 전체 fixture·전체 관문·실제 backend와 317 warm spare는 실행하지 않았다. 316의 PASS와 warm 판정을 317 결과로 승계하지 않는다.

## native

> **최종(⟦TBD:ship-date⟧):** 측정 기준은 Claude Code 2.1.317·Codex CLI 0.164.0이다(2026-11-03 무과금 재측정).
> 도움말·도구 집합은 316과 같고 desktop용 Backdrop plugin API가 추가됐으며, 6경로 표면 관문이 통과했다. 전체 fixture·전체 관문·실제 backend는 이번 평가에서 미실행이다.
> 출하 검증 대상 native는 2.1.317(`26b01ec2…6038`)이다. 직전 기준 316의 fixture·표면 결과는 위 "2026-10-31~11-01 추가"에 보존한다.
> 그 전 기준 2.1.314(`cb069eab`, 2026-10-30 재측정·반영)의 차이는 위 "2026-10-30 추가"에 적었다. 그 전 기준
> 2.1.312(`4b963405`, 2026-10-27 재측정·2026-10-28 반영)는 도움말·plugin API·도구 표면이 버전 줄 외에 같았다.
> 2.1.312·2.1.314·2.1.316 정적 판독에서 전용 `--bg-spare` pool은 Windows에서 spare를 만들기 전에 반환하지만,
> Agent View dispatch의 미리 띄우기 경로는 그렇지 않다. 그래서 세 버전의 Windows warm spare는 native 코드 가드를 이유로
> NOT_APPLICABLE로 판정하지 않는다(실행 관측 아님). 317의 별도 warm 정적 검토와 실행 관측은 미완료다.

> **2026-10-23 갱신:** 현재 측정 기준은 Claude Code 2.1.305·Codex CLI 0.163.0이다(`c6264df9`). 같은 날 무과금 재측정에서
> 도움말·plugin API·도구 표면은 버전 줄 외에 변함없고 fixture 묶음·표면 관문은 PASS였다. 아래 2026-10-22의 재측정 미완료
> 표시는 이 결과로 대체한다. 최종 출하 native·Codex CLI 버전과 native SHA, 최종 전체 관문·실제 backend 결과는 `TBD(gate)`다.

> **2026-10-22 갱신:** 아래 2.1.304·0.163.0은 2026-10-20에 기록한 측정 기준으로 보존한다. 이 값을 현재 출하 후보의
> 검증 완료 버전으로 읽지 않는다. 2.1.305는 경로표 추출 대상에 추가됐지만, 그 사실만으로 재측정이나 전체 관문이 통과한 것은 아니다.
> 현재 출하 기준은 이 갱신으로 대체하며, 최종 native·Codex CLI 버전, native SHA와 무과금 재측정 결과는 `TBD(gate)`다.

측정 기준(`measured-clients.json`)은 Claude Code 2.1.304, Codex CLI 0.163.0이다(2026-10-20 무과금 재측정). native는 고정하지 않는다. 아래는 그 전의 경과다. 처음 측정 기준은
Claude Code 2.1.299, Codex CLI 0.162.1이었다(2026-10-14 무과금 재측정). 설치본 native는
2026-10-15에 2.1.300으로, 2026-10-16 합성 관문 도중 2.1.301로 자동 갱신됐다. 실행 중 바이너리가 바뀐 관문은 무효로 남겼다.
2.1.301의 6경로 무과금 도구 표면 재측정은 통과했다. 도움말과 선택 UI API 선언의 차이를 확인했고, Agent View를 켠 구성에서는
기본 agent인 `claude`가 도구 목록에 나타났다. 제품의 측정 기준과 기준 스냅샷은 갱신하지 않았다. 동일 native를 검증 동안만
읽기 잠금으로 고정해 전체 관문을 실행했고, 마지막 검증 뒤 잠금을 해제했다. 전역 자동 업데이트 설정은 바꾸지 않았다.
2026-10-19부터 #319 마감 검증은 2.1.303만 대상으로 하며 같은 방식으로 읽기 잠금을 걸어 실행했다. Windows에서 2.1.302·2.1.303은
warm pool을 만들기 전에 반환한다(native의 코드 가드, [NATIVE-SUPPORT](NATIVE-SUPPORT.md)). 그래서 warm spare dispatch는 실행된
성공으로 보고하지 않는다.

## 출하 검사

출하 신원과 최종 결과는 바로 아래 "최종 출하 후보" 절에 있다. 그 아래 절들은 날짜별 이력이다.

### 최종 출하 후보 (⟦TBD:ship-date⟧, `⟦TBD:tag-commit⟧`)

- **최종 로컬 전체 관문:** ⟦TBD:full-gate⟧ — gofmt·vet·build, 전체 Go·race, 합성·TUI, 문서 인용.
- **provider-control probe·CTL v2 controller:** v0.6.11은 provider-control probe 8 entry ADMITTED와 CTL v2 controller campaign(pilot → 단일 세트 → N=5) 없이 출하한다. campaign은 실행하지 않았다. 제품 launch는 admission을 사용하지 않는다(#319 decision 17). R04L은 native 2.1.316 무과금 진단(같은 후보의 MULTI_TURN 3 run)에서 INCONCLUSIVE이고, RESUME_EXPLICIT·CONTINUE_PRINT·TUI·RP01 scope는 실행하지 않았다. [#340](https://github.com/wotjr1649/Clauduct/issues/340)이 이어받는다. `0248efbc4`(같은 chunk의 upstream 오류와 downstream 실패)는 실제 backend PASS 없음(offline real-HTTP 회귀만 있음)이다.
- **실제 backend:** ⟦TBD:live-18⟧ — 18개 회귀 묶음, 변경 영향(`9d878bbb`의 `--background-stop` 오류 문구 포함)과 TUI,
  backend 생성 한도 capability 재측정.
- **routing proof 대기·종료 중 503·요청 진단 필드의 실제 backend 판정:** ⟦TBD:runtime-317⟧ — proof 대기(2초를 넘겨 5초 안에
  확인된 표본), gateway 종료 중 503 `GATEWAY_CLOSED`와 hook의 종료 처리, 요청 기록의 진단 필드 형식(`f38cf2b6`·`e57723e7`).
  항목마다 PASS·FAIL·NOT_EXERCISED로 적고, NOT_EXERCISED는 실제 backend PASS로 보지 않는다.
- **무과금 재측정:** native 2.1.317(SHA-256 `26b01ec2…6038`), 2026-11-03. Codex CLI는 설치본 0.166.0을 관측했고 도움말이 같아
  측정 기준 0.164.0을 유지했다. 도움말·도구 집합은 316과 같고 desktop용 Backdrop plugin API가 추가됐으며 6경로 표면 관문이 통과했다.
  전체 fixture·전체 관문·실제 backend는 이번 평가에서 미실행이다. 2.1.316(`e896a9d8…2d52`)의 2026-10-31 fixture·표면 PASS와 앞선
  314·312의 재측정은 과거 입력의 결과로 유지한다. 출하 검증 대상 native: 2.1.317(`26b01ec2…6038`).
- **출하 신원:** ⟦TBD:ship-hashes⟧ — `clauduct.exe`·`clauduct-probe.exe`·`install.ps1`·`uninstall.ps1`·`SHA256SUMS`의 SHA-256,
  깨끗한 태그에서 Go 1.27.2·`CGO_ENABLED=0`·`-trimpath`로 만든 두 빌드의 일치, 내장 probe digest ⟦TBD:probe-digest⟧.
- **설치·업데이트·되돌리기:** ⟦TBD:install-check⟧ — 제품과 probe의 짝, 새 설치, v0.6.10에서 교체·되돌리기, 설치본 실행과
  재업데이트 무변경.
- **앞선 `TBD(gate)` 정산**
  - 버전 고정 없는 신원 확인 보완(2026-10-22)과 capture 대기 판정(`5d36c9b0`)·PR #330 리뷰 수정(`abc6e56b`·`e6eabfd4`·`7a12005e`·
    `4aa8b3a7`·B10 `fb66791b`)은 이 후보에 들어 있고 위 관문·실제 backend·설치 결과가 그 검증이다. 자동 갱신 중 동작: ⟦TBD:autoupdate-identity⟧.
  - 경로 template 보류 4개는 출하 전에 재실행하지 않았다. HOLD로 남고 [#336](https://github.com/wotjr1649/Clauduct/issues/336)가 이어받는다.
  - #325 관측기 고부하·종료 검증: ⟦TBD:obs-325⟧.
  - Go 1.27.2 최종 빌드·재현성과 probe digest는 위 "출하 신원"이다.

### 현재 출하 후보 — 2026-10-22 갱신

> **2026-10-23 갱신:** 이 확인 목록은 유지한다. 아래 무과금 재측정의 미완료 표시는 2.1.305 재측정 PASS로 대체하되,
> 최종 후보의 신원과 일치하는지 확인하는 출하 관문은 `TBD(gate)`로 남긴다. probe digest 갱신도 출하 신원 확정은 아니다.

다음은 이전 후보의 기록을 덮어쓰지 않는 새 확인 항목이다. 최종 후보·릴리스 바이트로 확인한 뒤 채운다.

- **최종 로컬 전체 관문:** `TBD(gate)` — gofmt·vet·build, 전체 Go·race, 합성·TUI 및 문서 인용 결과.
- **실제 backend:** `TBD(gate)` — 회귀 묶음, 변경 영향과 TUI, backend 생성 한도 capability 재측정 결과.
- **무과금 재측정:** `TBD(gate)` — 최종 native 신원, 도움말·hook·도구 표면과 fixture 결과.
- **출하 신원:** `TBD(gate)` — 출하일, `v0.6.11`의 최종 commit, `clauduct.exe`·`clauduct-probe.exe`·`install.ps1`·
  `uninstall.ps1`·`SHA256SUMS`의 SHA-256. 깨끗한 태그에서 Go 1.27.2·`CGO_ENABLED=0`·`-trimpath`로 만든 두 빌드의 일치도 확인한다.
- **설치·업데이트·되돌리기:** `TBD(gate)` — 제품과 probe의 짝, 새 설치, v0.6.10에서 교체·되돌리기, 설치본 실행과 재업데이트
  무변경. v0.6.10 이하의 `--update`는 제품만 교체하므로 probe가 필요하면 새 제품으로 한 번 더 업데이트한다
  ([PACKAGING](PACKAGING.md) 5.1절). 출하 절차는 같은 문서의 4.1절을 따른다.

아래의 "마지막 실행"·"현재"와 제품 소스 표시는 각 절의 날짜에 해당한다. 이번 출하의 최종 결과는 위 "최종 출하 후보" 절이 소유한다.

### #319 마감 관문 (2026-10-20, HEAD `124f1f76`, 제품 동작 소스 `5e6de560`, native 2.1.303)

- **전체 관문은 실패했다.** 마지막 실행 r5는 13단계 중 12단계가 종료 코드 0이었다. f123 단계의 1행(W0 F1-real-move)이
  OS 송신 관측기의 불완전 capture로 실패했다.
  - 관측기 기록: 미귀속 대기열 overflow 498,676건, ETW 이벤트 유실 24,943건, 해석 실패 0건이다.
  - 그 행의 관측 구간에 host의 git 다운로드 급증(약 29,500 packet/s)이 겹쳤다.
  - 같은 행을 조용한 host에서 단독으로 다시 돌린 결과는 PASS였다. 관측은 완전했고 overflow·유실은 0건이었다.
  - 이 단독 PASS는 그 한 행만 대신한다. 전체 관문 PASS로 보지 않는다.
- **앞선 시도의 실패도 보존한다.**
  - r1: 관측기의 ETW Fail(17) 해석 결함과 L08 시험 조건 결함. 둘 다 수정했다.
  - r2·r3: host의 다른 프로그램 트래픽이 관측기 대기열을 넘쳤다.
  - r4: gateway 시험이 스트림 끝을 기다리지 않고 집계를 읽었다. 시험만 수정했다.
  - 제품 코드는 r1 이후 바뀌지 않았다.
- **관측기 한계:** 관측기는 소유자를 아직 모르는 host 전체 트래픽을 5초 동안 최대 32,768건 보관한다. 그래서 다른 프로그램의
  지속 트래픽이 크면 그 행을 FAIL로 끝낸다. PASS로 넘기지 않는 것이 의도다. 고부하 내성 개선은 후속 과제다.
- **합성 행 집계(104행, 원래 PARTIAL·NOT_RUN):**
  - 결과는 PASS 68, PARTIAL 34, NOT_APPLICABLE 2, FAIL 0이다.
  - NOT_APPLICABLE 2행은 Windows warm spare다. native 코드 가드라 실행된 성공으로 보지 않는다.
  - PARTIAL의 주된 원인은 OS 송신자 귀속 공백, 대조군 미재현, 일부 표면의 미완료 증명이다. 해당 행은 F123, R30x, A06, Q01 등이다.
  - 기존 엄격 판정기는 전체 관문 실패와 미해결 행 때문에 FAIL을 낸다.
- **실제 backend:** native 2.1.303과 `5e6de560`의 clean 후보를 저장소 밖 작업 폴더·격리 프로필·일반 권한에서 실행했다. 18개
  묶음이 모두 통과했다. 묶음은 foreground, 권한 묻기·Always·Deny, background 차단 attach, auto 분류기의 허용·실제 block·
  fail-closed, dispatch 선택, 저장 Reply, 정지 job 복원과 background, recap, resume, fork, 콘솔 닫기, branch, hook 재연결이다.
  - 그대로 둔 원래 판정이 셋 있다. background 묶음의 원래 판정은 재개 뒤 worker 교체를 확인하지 못해 FAIL이었고, 같은 job의
    정지 복원을 합친 판정이 PASS다. branch는 생성 단계와 재개를 합친 판정이 PASS다. dispatch는 원래 판정이 기본값 추가로 FAIL이었고,
    네 조합의 실제 model·effort와 설정 변경 없음을 확인한 정규화 판정이 PASS다.
  - 저장 Reply의 첫 시도는 새 작업 폴더가 신뢰되지 않아 요청 없이 실패했다. background 묶음과 같은 신뢰 수락 단계를 더한 실행이
    통과했다.
  - TUI `/model` 선택은 선택한 model이 다음 요청에 그대로 쓰인 것을 확인했다. 앞선 두 시도는 하네스 문제로 실패했고 기록으로
    남겼다(선택 목록 순서, 격리 프로필 설정을 건너뛴 실행 옵션).
  - teammate에게 두 번째 메시지가 전달됐는지와, 저장 뒤 재개한 요청에 이전 reasoning이 다시 실렸는지는 현재 진단이 기록하지
    않아 실측으로 확인하지 못했다. 후속 과제다.
  - 실제 backend가 모두 통과해도 합성 행의 PARTIAL과 전체 관문 실패는 그대로다.
- **문서 인용 검사**: 공개 문서 40개와 비공개 문서를 포함한 인용·링크 검사가 종료 0, 실패 0이었다.

### 이전 기록 (2026-10-17)

- **로컬(과금 없음, HEAD `4813b7ae`, 제품 동작 소스 `599da41b`, native 2.1.301)**
  - 이전 소스 `8a14e1e5`의 native 고정 전체 관문은 실패했다. vet 7구성, Go 전체 시험, fixture 2구성, 구조 검사 12묶음,
    전체 race, foreground·background 4구성은 종료 코드 0이었지만 Agent View와 수명 합성 검사가 실패했다.
  - 새 job의 프로필 등록 전에 부모 세션이 해제되는 순서 문제를 재현하고 수정했다. 회귀 시험은 수정 전 실패, 수정 후와 race에서
    통과했다. 수정한 `6f0cee27`의 집중 재검증과 새 전체 관문은 통과했다. 전체 관문은 vet 7구성, Go·race 전체,
    fixture 2구성, 구조 검사 12묶음, foreground·background 4구성, Agent View와 수명 합성 검사를 포함했다.
    합성 행의 PARTIAL·NOT_RUN·INCONCLUSIVE는 그대로이며, 관문 성공이 모든 지원 범위의 실증을 뜻하지 않는다.
  - 후속 `599da41b`는 backend 표시 metadata 처리를 더했다. protocol·bridge 전체 시험, gateway 집중 시험과 해당 영역 race가
    통과했다. 이 소스의 전체 관문은 2026-10-17 00:23부터 04:03까지 실행해 실패했다. 마지막 F123 19행 중 launcher 종료 행에서 resident/readiness·attach가 실패했고, 18행은 PARTIAL이었다. 다른 단계는 종료0이었다. 이 실패는 보존한다.
  - `4813b7ae`는 검증용 driver를 보정했다. launcher만 종료한 시험에서 driver가 약 1초 뒤 콘솔도 닫았으며, 진단 실행에서
    bridge와 native가 콘솔 닫기 약 42 ms 뒤 함께 종료됐다. 실제 ConPTY 대조 시험은 수정 전 실패했고, 수정 후에는 관측 완료
    신호까지 콘솔을 유지했다. 신호 검증·실패 보존·종료 후 입력 차단을 포함한 15사례는 race에서 통과했다. 제품의 1초·5초·60초·75초
    검증 조건은 유지했고, launcher 행에 75초 관측까지 콘솔 유지 조건을 더했다. 해당 행과 close-midmove의 집중 재검증은 통과했다.
    별도 real-close 집중 실행에서는 답변 뒤 native UI 종료 실패를 한 번 관측했고, 같은 코드의 새 실행은 통과했다. 이 실패 원인은
    미확정으로 보존한다. **보정 후 첫 전체 관문은 resident relay race 회귀 1건으로 실패했다.** 다른 단계는 종료0이고 F123 19행의 candidate는14PASS·1INCONCLUSIVE·4대조 행이며 증거 판정은 모두PARTIAL이다. close3행의 assertion은 모두 충족됐다. relay 관련4시험은 변경없는 일반·race 분리 재검증에서 통과했지만 최초 실패 원인은 미확정이며, 다음 전체 시도는 일반 Go에서 같은 시험이 다시 실패해 중단했다. 별도 app 일반 진단은 통과했지만 대상 반복 측정은 첫 시도에 오류 반환 없이 외부500ms 조건에서 실패했다. 내부 relay 성공은 이 실패를 대체하지 않으며, 기존 exit status1과의 인과와 각 원인은 미확정이다. race 반복은 미실행이고 한도·판정은 유지했다. 공개 제품의 의존성245개에는 이 검증 driver가 없다.
  - print Ctrl+C의 child 종료를 콘솔 신호보다 먼저 받는 회귀 시험은 수정 전 실패, 수정 후 통과했고 전체 Go·race에서도 통과했다.
  - 이전 관문의 hook deadline 초과는 원인 미상이다. 시간 한도와 판정은 유지한 채 child 시작 시각 진단을 보강했으며,
    집중 시험과 `8a14e1e5` 전체 Go·race에서는 통과했다. 통과한 재실행만으로 최초 원인을 확정하지 않는다.
  - Agent View 공개 변경과 후속 수정은 독립 리뷰와 재리뷰의 지적을 반영했다.
- **실제 backend**
  - Agent View 공개 전 후보(2026-10-14, native 2.1.299)의 실제 묶음 2차에서 resume, fork(원본 불변), 저장 Reply의 wake, 정지 job
    직접 attach, hook 재연결, Deny·Esc 취소가 통과했다. 그 묶음은 작업 폴더가 저장소 안이어서 native가 상위 폴더의 지시 파일을
    project instructions로 읽었다(native 정상 동작). 이번 묶음은 저장소 밖에서 실행했다.
  - auto 모드 분류기의 실제 block과 권한 Always는 관측하지 못했다.
  - `6f0cee27` 후보의 원래 foreground 재개는 알려지지 않은 `response.metadata`를 거부해 실패했다. 실패한 event의 원문은
    수집하지 않았다. 별도 실제 요청에서 표시 metadata 형태를 확인하고 `599da41b`로 처리한 실제 요청은 정확한 답·routing·완료를
    확인했다. 이전 실패 event와 별도 요청의 세부 형태가 같았다고 단정하지 않는다.
  - 현재 `599da41b`의 clean checkout에서 Go 1.27.2·`CGO_ENABLED=0`·`-trimpath`로 만든 검증 전용 공개 후보
    (`vcs.modified=false`, 태그 없음)를 저장소 밖 작업 폴더·격리 프로필·일반 권한에서 실행했다. foreground, `/background`
    이동 후 원래 대화 재개와 같은 job attach, dispatch, fork, 빈 agents 화면이 통과했다. fork의 원래 대화 항목은 같았고,
    native 관리 기록 때문에 원본 파일의 바이트는 바뀌었다. 선택한 dispatch의 main 요청은 sol/high였으며 보조 모델 요청도 기록했다.
  - 콘솔 닫기 근사는 같은 job의 복구·대화 응답을 확인했지만, 하네스가 native supervisor 종료 전에 잔여 0을 기다려 정리에
    실패했다. 정상 job 삭제와 해당 시험 profile의 정상 supervisor 종료로 잔여 0을 확인했다. 기존 실패 기록을 보존하고,
    대상 소유권을 검증하는 하네스 정리를 보완한 새 격리 시험은 통과했다. 세 프로세스의 종료 적용, 60초 대기 뒤 같은
    job·session·link·port 복구, 정확한 답과 이력을 확인했고 정상 정리 후 잔여는 0이다. 앞선 정리 실패 기록은 보존했다.
    supervisor 종료를 포함한 정리이므로 제품의 독립 종료 성공으로 해석하지 않는다.
    콘솔 닫기 근사는 `CTRL_CLOSE_EVENT` 전달을 증명하지 않는다.
- **문서 인용 검사**: 이 초안과 공개 문서39개를 포함한 인용·링크 검사가 종료0·실패0이었다.
- **출하 빌드·태그·Release·설치**: 소유자 지시가 있어야 한다. 아직 없다.

> **2026-10-22 갱신:** 바로 위 "소유자 지시가 있어야 한다. 아직 없다"는 2026-10-17 당시의 기록이다. 현재 출하 신원과
> 실행 결과는 앞의 "최종 출하 후보" 절에 있다. 이 문서 갱신 작업은 병합·태그·Release·설치를 수행하지 않는다.

## 알려진 제한

- Agent View로 옮기는 도중 native가 job 상태를 만든 직후, native의 daemon·worker가 생기기 전에 콘솔을 닫으면 job은 worker 없이
  남는다. native 단독에서 같은 순간에 닫은 경우와 같다. job 상태가 생긴 뒤 record까지는 측정에서 51 ms와 58 ms였다. 다음 Clauduct
  실행은 정상으로 진행한다.
- Agent View에서 새로 시작한 작업과 Agent View가 미리 띄우는 세션은 native 설계대로 Clauduct의 `--settings`를 이어받는다.
  합성 측정에서 미리 띄운 세션은 요청을 보내지 않았고 Clauduct보다 먼저 끝났다.
- attach나 respawn으로 이어 간 job은 `clauduct rm` 직후에는 record와 events 디렉터리가 남는다. 같은 연결의 마지막 job을 삭제한
  측정에서는 연결 유지 프로세스가 약 3초 안에 회수했다(합성 측정 2.6초). 같은 resident를 공유하는 다른 job이 남으면 보존한다.
- fork 부모를 증명하지 못하면 그 fork는 `SESSION_PROFILE_UNVERIFIED`로 거부된다.
- 2.1.301의 `-p --continue` 합성 행은 `SESSION_RESTART_REQUIRED`로 거부됐고 두 번째 추론 요청이 없었다. 같은 구성의 명시적 `--resume <UUID>`는 다음 요청을 처리했다. `--continue`의 연속 요청 성공은 확인되지 않았다.
- 측정 기준은 2.1.304, #319 마감 관문의 native는 2.1.303이다. 설치본 native가 자동 갱신되면 doctor는 `re-measure due`를 보일 수 있다.
  - **2026-10-22 갱신:** 위 2.1.304는 기록된 기준이다. 현재 출하 기준·2.1.305 재측정 여부는 "native" 절의 `TBD(gate)`로
    대체해 관리한다. #319의 2.1.303 결과는 해당 과거 후보에만 적용한다.
- #325 관측기 보강과 경로표 결정의 최종 반영·재실행은 `TBD(gate)`다. 보류 경로의 유지와 기록용 경로 채택은 통신 허용을 뜻하지 않는다.
  - **2026-10-23 갱신:** 위 재측정 미완료와 경로표 반영 미완료 표시는 각각 2.1.305 무과금 재측정 PASS와 `028cd1ce`의
    반영 사실로 대체한다. #325 검증 보강은 "2026-10-23 추가"에 기록했다. 최종 관문·고부하 검증과 보류 4개 경로의 재실행은
    계속 `TBD(gate)`다. #319의 2.1.303 결과는 해당 과거 후보에만 적용한다.
- **출하 시점(⟦TBD:ship-date⟧):** 위 두 항목의 최종 상태다. 측정 기준은 2.1.317·Codex CLI 0.164.0이고 출하 검증 대상 native는
  2.1.317(`26b01ec2…6038`)이다. 보류 4개 경로는 재실행하지 않아 HOLD로 남는다([#336](https://github.com/wotjr1649/Clauduct/issues/336)). #325는 ⟦TBD:obs-325⟧.
- 관리 명령·`--bg` argv 판정 차이([#331](https://github.com/wotjr1649/Clauduct/issues/331))와 복원한 resident가 hook 트리·job
  종료와 함께 끝나는 것([#332](https://github.com/wotjr1649/Clauduct/issues/332))은 v0.6.11 범위 밖이며 COMPATIBILITY 원장에 있다.
- 세션 중 `/model`로 바꾼 model·effort pair가, resident가 다시 만들어진 뒤 그 세션을 resume하면 시작 pair로 돌아갈 수 있다.
  코드 경로를 읽어 판단했고 실행해서 재현하지는 않았다. v0.6.11은 이를 알려진 제한으로 두고 출하한다(소유자 결정 2026-10-31).
  수정과 그 실제 backend 검사는 [#341](https://github.com/wotjr1649/Clauduct/issues/341)이 이어받는다.
- 같은 chunk의 upstream 읽기 오류와 downstream 전달 실패가 겹치는 분기(`0248efbc`)는 실제 backend PASS 없이 문서화된 한계로
  출하한다("2026-10-30 추가", [#342](https://github.com/wotjr1649/Clauduct/issues/342)).
- provider-control probe 8 entry ADMITTED와 CTL v2 controller campaign 없이 출하한다("최종 출하 후보", [#340](https://github.com/wotjr1649/Clauduct/issues/340)).
- 그 밖의 남은 제한은 [COMPATIBILITY.md](COMPATIBILITY.md) 3절에 있다.
