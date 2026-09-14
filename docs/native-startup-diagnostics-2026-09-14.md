# Native 시작 진단 조사 — 2026-09-14

최신 `origin/main`을 fetch한 결과 `4f3e37535075b662de8d23cfed8f6bf8e23f3933`이었다. 이 커밋에서 `fix/native-startup-diagnostics`와 별도 worktree `.tmp/native-startup-diagnostics`를 만들었다. 산출물은 로컬 진단 개선 후보이며 **전체 검증 PASS 또는 실사용 해결로 판정하지 않는다**.

## 보존한 상태와 사용자 증거

원래 루트의 README 미커밋 변경 SHA256은 `38f3d4252880099592e73987c81c04f60c93e787ca482aaf185f2f4bd1496c08`이며 기존 untracked/구현/증거/비용 원장을 보존했다. 별도 detached baseline은 새 작업 worktree의 `.tmp/baseline`이다. 원격 push/merge/Release, 설치본 교체, 전역 설정/auth/native binary 변경은 이번 작업에 포함하지 않았다.

인계 문서의 사용자 보고: v0.1.0 설치본에서 `clauduct --model luna --effort low --print "hello"`가 exit0, upstream 2요청 모두 HTTP200/completed, failed0, cleanup 모두 true였다. 동시에 `unrecognized_model`과 `transportRejections=2`가 있었다. 토큰 수는 미관측이다. 개인 status/session 원문을 추가로 읽거나 fixture로 복제하지 않았다.

PowerShell7 및 다른 머신 PowerShell5.1의 설치 성공은 사용자 제공 증거다. 이전 개발 머신5.1의 정책 거부와 구별하며, 과거 배포 snapshot이나 무인 goal 판정을 변경하지 않는다.

## transportRejections: 원인, 영향, 수정

기존 다섯 HTTP 이벤트 handler는 사건 합계만 증가시켜 원인을 버렸다. 모델 요청 실패 카운터가 아니므로 기존 `2`를 backend 실패2회로 해석할 수 없다. 이벤트 종류, 연결 중복, 발신자와 목적은 과거 합계에서 복원할 수 없다. 정상 탐색이나 무해한 오류라고 결론내리지 않는다.

최소 수정은 고정 event label별 합계, 허용 code별 clientError 합계, WeakSet을 이용한 서로 다른 socket 수다. 객체 key 수는 각각 최대5개/15개이며 원문 오류, rawPacket, 주소, 헤더, URL, body를 보관하지 않는다. 알 수 없는 code는 `OTHER`다. status API 및 종료 snapshot도 허용 목록으로 다시 투영하고 복사본을 반환한다. legacy의 미관측 필드와 새로운 관측0을 구별한다.

기존 `rejected`, `transportRejections`, `rejectedBeforeStart`, started/succeeded/failed 의미를 유지했다. CONNECT/Upgrade/clientError의 socket 파괴, Expect의417 응답, 인증·요청 검증은 유지한다. Node의 clientError는 부분 요청 뒤에도 발생할 수 있으므로 기존 설명의 “항상 요청 처리 전”을 수정했다. [Node HTTP 이벤트 설명](https://nodejs.org/docs/latest-v24.x/api/http.html#event-clienterror), [v24.19.0 HTTP server 구현](https://github.com/nodejs/node/blob/v24.19.0/lib/_http_server.js)을 대조했다.

공개 loopback fixture는 일반 HEAD `/api/hello`와 status 읽기에서 거부0을 확인했다. 같은 socket에 연속 Expect 요청을 보낸 사례에서는 사건2/연결1이었다. 이는 합계만으로 연결 수를 추정할 수 없다는 재현이며 사용자 hello의 두 사건을 재현했다는 뜻이 아니다.

## unrecognized_model: 발생 조건과 영향

[공식 오류 설명](https://code.claude.com/docs/en/errors#unrecognized-model-id-on-a-request)에 따르면 알려진 모델로 해석되지 않고 해당 identity로 매핑하는 modelOverrides도 없을 때 요청 ID별로 프로세스당 한 번 기록한다. `-p`의 query_source는 `sdk`다. 요청 ID를 그대로 전송하며 이 진단 자체로 종료하거나 모델을 전환하지 않는다. 인계의 출력과 일치한다.

[공식 모델 설정](https://code.claude.com/docs/en/model-config#correct-the-window-for-a-gateway-or-custom-model-id)은 custom ID의 컨텍스트 가정과 `CLAUDE_CODE_MAX_CONTEXT_TOKENS`를 별도로 설명한다. picker 등록이나 표시 metadata는 알려진 Claude identity가 되는 조건이 아니다. modelOverrides를 이용해 경고를 제거하는 방법은 Claude identity를 부여하므로 GPT에 적용하지 않았다. `behavesAs`도 복원하지 않았다.

설치된 `C:/Users/js/.local/bin/claude.exe`를 실행하지 않고 정적으로 확인했다. PE ProductVersion `2.1.270.0`, 내장 VERSION `2.1.270`, BUILD_TIME `2026-09-12T18:08:42Z`, SHA256 `fd7f35ec7761195ab5ba4eff423e48a78a7849e78f60d93ec31256cdb1a9ec7e`다. 아래 offset은 이 파일의 Latin1 byte offset이며 다른 버전에는 적용하지 않는다. minified 이름은 여러 모듈에서 중복되므로 이름만으로 검색한 다른 함수와 혼동하지 않는다. 바이너리 내용은 fixture로 복사하거나 실행하지 않았다.

| 정적 경로 | 관측 및 의미 |
| --- | --- |
| `i$e` / 경고 문자열, 196428932 부근 | model_validation과 미해결 Bedrock profile 일부를 제외하고, process 내 중복을 제한한 뒤 `IU`로 원 ID/정규화 ID를 검사한다. 미인식이면 경고를 기록한다. |
| `IU/je/zz/wi`, 191587599–191588700 부근 | 알려진 catalog identity, modelOverrides 등을 확인한다. GPT custom picker 이름 자체를 알려진 Claude로 만들지 않는다. |
| `aF/cF`, 191598374–191600114 부근 | 미인식 custom ID에는 MAX_CONTEXT_TOKENS가 적용될 수 있다. metadata나 별도 model context 분기가 앞선다. Clauduct는 기본400000과 AUTO_COMPACT_WINDOW400000을 이미 전달한다. |
| `nw/GB`, 197289546 부근; `T5n`, 197287935 부근 | 설정 창을 모델 창으로 제한하고 출력 예약량은 min(native 출력값,20000)이다. 기존320K 압축 목표의 설정 계산은 보존한다. 실제400K/320K 동작은 미검증이다. |
| `ul/iz/dl`, 191551856 부근 | native가 인식하는 runtime metadata에 input/output/effort/capabilities가 있다. 현재 gateway `/v1/models`는 id/object/owned_by만 제공하며 그런 metadata를 선언하지 않는다. 소비 여부와 실제 GPT 용량이 검증되지 않은 metadata를 새로 넣지 않았다. |
| `bX`, 191600765; `Bje`, 198773160 | 알려진 출력 metadata가 없으면 기본32000/상한128000 분기가 있다(`rF/jse`, 191597998 부근). env·실험·capability cache가 값을 바꿀 수 있어 사용자 hello에 실제 적용된 값이라고 단정하지 않는다. |
| `Jse`, 191602000 부근; `ey`, 192278723 부근; `d$n`, 191605372 부근 | capability override, metadata, model pattern/provider별 fallback 경로가 있다. 미인식이 모든 기능 비활성화를 뜻하지는 않는다. 실제 native feature 선택 전체는 미검증이다. |

Clauduct는 MAX_OUTPUT_TOKENS를 임의로 덮어쓰지 않으며 native가 보낸 max_tokens를 completion usage로 검증한다. 실제 초과 생성 자체를 사전에 막는 제한은 아니다(`usage-enforced-completion`). 모델/effort는 기존 route 및 prepareNative 검증을 거친다. 사용자 보고 luna/low 성공은 해당 두 요청의 증거이며 다른 기능 전체의 호환성 증거로 확대하지 않는다. 경고를 숨기는 stderr 필터나 Claude identity 위장, 기능을 추측해 켜는 수정은 하지 않았다.

## 실행한 검증과 남은 실패

환경은 Node `24.19.0`, 기존 `src/run-node-tests.ps1`의 깨끗한 환경/직렬 실행/60초 제한이다. 관련 코드는 공개 fixture와 loopback, task 내부 임시 파일만 사용한다. 새 실모델 요청0, 실제 credential 읽기0, 모델 입력/출력0이다. 공식 문서 조회와 git fetch는 실모델 요청이 아니다. 기존 비용 누적/미관측 예약은 변경하지 않는다.

최종 기능 검증 묶음의 원본 결과는 작업 worktree `.tmp/evidence/regression.json`이다. **24개 Node test entry 중21 PASS / 3 FAIL**, 약18.96초, runner timeout=false, test exit1이다. 기록용 PowerShell 호출의 exit0과 test exit1을 혼동하지 않는다.

결과 파일 SHA256은 `559a6bb1eb8cdc464948662b761c772d1649704c28e02a557d6f7affa79a8e6b`다. 이후 실패 assertion의 동일한 조건을 boolean/길이 비교로 표현해 실패 로그에 응답 원문이 실리지 않도록 했다. 기능 구현은 그대로이며 동일 실패의 반복 실행은 하지 않았다. 변경된5개 JavaScript 파일의 최종 `node --check`는 모두 exit0(`.tmp/evidence/syntax.json`), 최종 diff의 whitespace 검사도 통과했다.

- 이전 핵심14파일 모두 PASS: credential-recovery, credential-store-selection, collected-result-relay, workflow-resume, failed-completion-resume, completion-selection, workflow-selection, native-protocol, request-admission, fixture-token-budget, runtime-discovery, runtime-paths, install-check, launcher-native. 파일명은 `src/test-<name>.mjs`다. 기존 symlink/native Workflow/정상OAuth NOT_RUN은 유지한다.
- native-diagnostics의17개 확인 PASS.
- 새 transport-rejections의9개 독립 시험 중6 PASS: checkContinue, connect, upgrade, 동일 연결의 두 사건, 정상 hello/status, legacy/악성 status 투영 및 code 허용 목록. 정상/음성 검사를 mock event로 바꾸지 않았다.
- **checkExpectation FAIL**: Expect public marker를 보냈지만417 대신401, 거부 event map은 빈 객체였다. 독립 HTTP 서버 관측에서도 HTTP1.1이나 req.headers.expect가 없었고 수신 socket 바이트에 해당 public Expect 문자열이 없었다. `100-continue`는 수신 바이트와 checkContinue 모두 관측됐다. 중간 변경 주체는 확인하지 못했다. 보호나 네트워크 설정을 바꿔 재현을 강제하지 않았다.
- **invalid-method FAIL**: gateway의 무응답 socket 종료를 기대했으나 client가4096byte 상한을 넘는 응답을 받아 FIXTURE_OUTPUT_LIMIT이었다. 응답 원문은 기록하지 않았다. 발신자/내용을 추정하지 않는다.
- **header-overflow FAIL**: 앞선 분리 실행에서는 HPE_HEADER_OVERFLOW를 확인하고 PASS했으나 최종 묶음에서는 client의2초 제한으로 FIXTURE_TIMEOUT이었다. 안정적 PASS로 판정하지 않는다.
- 기존 native-gateway 전체 검사는 수정본에서20초/30 checks 뒤 timeout, 별도 수정 전 main baseline에서도20초/22 checks 뒤 timeout이었다. 새 회귀라고 확정할 수 없으나 전체 PASS도 아니다. watchdog·검증 조건을 늘리거나 제거하지 않았다.

독립 HTTP 관측은 `.tmp/test-http-observation.mjs`에 남겼다. 이것은 사건 관측 도구이며 exit0이 HTTP 전달 성공을 뜻하지 않는다. 실제 두 경고를 다시 발생시킨 native 실행, 기본400K/320K, 설치26검사 재실행은 하지 않았다. 설치 코드 변경은 없다.

후속 확인에는 새 진단이 포함된 실제 native 실행의 허용된 요약과, public wire가 변형되지 않는 것으로 확인된 환경의 strict regression 결과가 필요하다. 실제 모델 호출의 범위·비용 상한은 이번에 새로 부여되지 않았다. 현재 증거로 원래 두 거부의 목적·발신자를 확정하거나 경고 해소/배포 준비 완료를 주장할 수 없다. 실패를 유지한 로컬 커밋은 검토용 후보이며 원격 배포 승인이 아니다.

## 후속(2026-09-14): 세 FAIL의 원인은 loopback HTTP filter였다

원인은 이 개발 머신의 AdGuard가 loopback HTTP를 가로챈 것이다. 보호를 끄자 transport-rejections는9/9 PASS, 공식 실행기 구성(`--test-concurrency=1`)의15파일 묶음은23/23 PASS, 앞서20초 watchdog에 걸리던 native-gateway 전체 검사도62 checks 완주했다. 확정 근거는 세 가지다. raw TCP listener가 받은 byte에서 비표준 `Expect` header만 사라졌고, 잘못된 method 요청에는 AdGuard의 `blocking-pages` HTML400이 돌아왔으며(32,879~171,633byte), filter가 닿지 않는 named pipe 대조군에서는 셋 다 기대대로 동작했다. 주소(127.0.0.1/127.0.0.2/`::1`)와 port15종 전부 가로채져 code 수준 우회는 불가능하다.

**판별법**: 로컬 HTTP 검사가 `401 LOCAL_SESSION_REQUIRED`나4096byte 초과 응답으로 실패하면 loopback HTTP filter를 의심한다. 이때 실패는 test가 의도적으로 보내는 기형 HTTP(잘못된 method, 비표준 `Expect`,17000byte header)에 filter가 반응한 것이며 gateway의 결함이 아니다.

**실사용 경로는 영향이 없다.** filter를 켠 상태에서 측정했다. chunked SSE stream은8/8 frame이200~207ms 간격으로 도착했고 변형·Content-Length 주입이 없었다. heartbeat 주기15초를 넘기는16초 간격 유휴 stream도3/3 도착에 socket error 없이 정상 종료했다. filter가 건드리는 것은 기형 요청뿐이다.

`close()`의 산발적 약1000ms는 `http-close.mjs`의 peer-drain deadline이며, filter가 peer의 FIN 전달을 지연시킨 결과다. 보호를 끄면 발화가0/10으로 사라진다. gateway 결함이 아니다.
