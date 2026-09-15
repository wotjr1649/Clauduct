2026-09-14 Clauduct 무인 출하 작업 브리핑

**작성 후 사용자 기준 변경:** 정상 인증 갱신은 사용자 Codex CLI 로그인 기준의 확인으로 분리하고 Clauduct의 갱신 판정·구현·검증을 출하 필수에서 제외했다. 기본 400K/320K 압축은 사용자 실사용 검증으로 이관하고, 4h/24h×3/72h 장기 시험은 제외했다. [진행 기록의 최신 기준](unattended-release-progress.md)에 반영했다. 아래 내용은 중단 시점의 작업·증거와 당시 명세에 대한 브리핑으로 보존한다. 제외된 세 항목은 더 이상 출하 보류 사유로 사용하지 않는다. 나머지 적용 가능한 필수 기능과 실패·복구·릴리즈 검증을 충족하면 변경된 범위의 로컬 출하 판정이 가능하며, 현재 후보가 이미 통과했다는 뜻은 아니다.

사용자의 “현재 작업만 마무리하고 지금까지의 작업을 정리하라”는 요청에 따라 **현재 수정 건을 마감하고 새 개발을 중단했다. 전체 출하는 HOLD이며 목표는 미완료다.** 마지막 수정은 `6c728af`에 커밋했고 로컬 ZIP과 해당 변경의 검증을 마쳤다. 다음에 예정했던 gateway 종료 지연 조사는 착수하지 않았다. 이 브리핑을 만드는 동안 새 모델 호출, 개발 시험, 릴리즈 생성은 하지 않았다.

브리핑 마감 당시 목표 도구는 `active`로 남겼다. 이후 두 자동 계속 turn에서도 개발 재개를 허용하는 새 사용자 지시가 없어, 최초 중단 요청을 포함한 세 turn의 권한 차단을 확인하고 목표 도구를 **`blocked`로 변경했다.** 전체 목표를 `complete`로 표시하지 않았다. 추가 개발·시험·실제 호출 없이 도구 상태와 기록만 정리했다. 차단 감사 기록(`../.tmp/unattended-release/user-stop-20260914/goal-blocked-audit.json`), 중단 기록(`../.tmp/unattended-release/user-stop-20260914/status.json`), [진행 기록](unattended-release-progress.md)에 현재 상태를 남겼다.

한두 개의 마지막 테스트만 남은 상태는 아니다. **정상 인증 갱신, 기본 설정의 반복 압축, 일반 프로젝트에서의 장기 무인 개발이라는 핵심 경로가 아직 완성되지 않았다.** 세부 복구 검증과 관리기 확장에 많은 작업을 했지만 이것을 전체 제품 완료와 혼동해서는 안 된다.

작업 범위와 보존 상태는 다음과 같다.

| 항목 | 마감 상태 |
|---|---|
| 원래 작업 루트 | `D:\AIDEV\Clauduct`, branch `fix/native-completion-resume`, HEAD `aa317c75c7bad0cf9d16641ca7db4edb43016c19` |
| 구현 위치 | `D:\AIDEV\Clauduct\.tmp\unattended-release\implementation` |
| 구현 branch / HEAD | `work/unattended-release-2026-09-13` / `6c728afa39bd5d5166dd82ba981d6b366fe03580` |
| 구현 Git 상태 | tracked clean, staged 0, 실행 증거가 든 untracked `.tmp/` 보존 |
| 원래 사용자 상태 | README의 기존 2줄 추가와 기존 untracked 파일 보존. 구현을 원래 branch에 통합하지 않음 |
| 배포 상태 | 로컬 후보 ZIP 생성. push, 외부 게시, 배포 없음 |
| 현재 실행 | 직전 산출물 검사에서 소유 확인 149범위의 잔여 0. 이번 중단 확인에서도 작업 경로에 일치하는 프로세스 0 |
| 장기 단계 | luna/max, sol/low 모두 `NOT_STARTED` |

이번 중단 확인에서 처음에는 작업 경로가 명령줄에 들어간 `pwsh.exe` PID 14860을 관측했다. 당시 다른 읽기 전용 검사도 동시에 실행 중이었다. 그 검사 종료 후 순차 재확인한 `2026-09-13T22:58:57.6792126Z`에는 해당 PID가 없고, 작업 경로의 일반·슬래시·JSON escape 표기에 일치하는 프로세스도 없었다. 관찰용 프로세스였을 가능성은 있지만 명령줄 원문을 수집해 확정하지 않았다. 별도 종료 명령을 실행하지 않았고 다른 Codex/Node 프로세스를 건드리지 않았다. 이 검사의 범위는 작업 경로와 직전 소유권 증거이며 시스템의 모든 프로세스에 대한 주장으로 확대하지 않는다.

전체 설계에 대한 현재 판단은 **전면 폐기·재작성을 해야 한다는 근거는 없지만, 현재 경로만으로 목표가 이미 성립하는 것도 아니다**이다. `Claude Code → loopback gateway → Codex backend 직접 HTTPS`의 변환·전달·resume 부분은 실제로 작동했다. 반면 정상 인증 갱신 주체를 연결하지 않았고, 임의 Bash/MCP 효과를 모두 조회·대조하는 계층도 없다. 필요한 최소 방향은 작동하는 변환 경로를 유지하면서 정상 갱신의 책임과 일반 프로젝트의 효과 대조·완료 판단을 완성하는 것이다. 이번 중단 이후 그 구현에는 착수하지 않았다.

누적 변경은 기준 `aa317c7..6c728af`에서 **41개 커밋, 168개 파일, 12,513줄 추가 / 225줄 삭제**다. 분류는 파일 경로 기준이며 `src/run-node-tests.ps1`은 테스트에 포함했다. 변경 파일 전체 목록(`../.tmp/unattended-release/user-stop-20260914/changed-files.tsv`)과 41개 커밋 전체 목록(`../.tmp/unattended-release/user-stop-20260914/commits.tsv`)에 빠짐없이 기록했다.

| 분류 | 파일 수 | 추가 / 삭제 | 의미 |
|---|---:|---:|---|
| 실행 코드 | 14 | 699 / 151 | transport, gateway, 대기, 요청 상태, 모델·Workflow 선택 등 |
| 검증기·개발 관리기 | 68 | 6,560 / 36 | 실행 소유, 사용량 예약, 과제·산출물 대조, 중단 복구, 실행 fixture |
| 테스트·러너 | 83 | 5,117 / 30 | 정상·실패·변조·중단 경계 검사 |
| 커밋된 문서 | 3 | 137 / 8 | 검증과 출하 한계 기록. 이 브리핑과 루트의 진행 문서는 이 통계 밖 |

검증기·관리기·테스트가 151/168개 파일이며 추가 줄의 약 93.3%다. 이 비율은 작업 시간의 비율이 아니고, 관리기 코드도 필요한 구현이다. 다만 핵심 사용 경로를 닫기 전에 이 계층의 세부 경계와 매번의 ZIP 재검증을 계속 확장한 **담당 agent의 우선순위 판단이 지연 원인**이었다. 환경 차단이나 사용자의 요구만으로 설명할 수 없다.

브리핑 중 목표 도구 관측은 누적 `112,690초`(31시간 18분 10초), `14,677,243 tokens`였다. 이는 호스트 목표 도구의 작업 집계이며 Clauduct가 backend에서 소비한 토큰이나 청구액과 같은 수치가 아니다. 브리핑 작성 이후의 집계 증가는 포함하지 않는다. 정확한 전체 완료율이나 남은 구현 일수를 계산할 근거는 없다.

지금까지 구현·검증한 내용은 다음과 같다. “실제 backend”, “실제 native + 공개 loopback 응답”, “로컬 단위 검사”를 구분했다. 과거 후보의 PASS를 최신 후보의 전체 PASS로 옮겨 적지 않았다.

| 작업 묶음 | 완료한 내용과 관측 | 아직 그 증거로 말할 수 없는 것 |
|---|---|---|
| 통신·재시도·대기 | Retry-After 숫자·날짜 처리, 60/300초 대기, 긴 대기의 영속 WAITING 및 재시작 후 보존. 메모리 admission의 기본 30초 기한. DNS/TLS/접근 오류 분류. 검색 재시도를 429/5xx로 제한 | 모든 실제 장기 장애 복구와 gateway 종료 문제 해결 |
| 인증·사용량 경계 | 취소·예산 소진 뒤 credential 재조회/전송 방지, credential store TOML 구조 판독, 요청·사용량·추가 rate-limit window 관측 | 정상 OAuth 갱신 구현, 실제 계정 잔여 quota의 확정 |
| 응답·출력 | 빈 type-only keepalive 지원을 좁게 추가하고 잘못된 필드·중복 type·순서를 거부. 출력 크기 제한, pipe/EOF와 완료 구분. 출력 단절 후 같은 세션 재개 검증 | 처음 실패했던 미보존 payload의 완전한 재현, 전체 TCP/취소 결함 해결 |
| 모델·자식·Workflow | 두 조합의 실제 메인·Agent 자식·신규 inline Workflow 호출 PASS. 긴 journal의 제한된 순차 읽기와 transcript 출처 확인. 연속 완료 증거 소비 및 메인 조정 SendMessage/TaskOutput 경로 PASS | 직접 부모 알림, 중첩/custom/cache-miss Workflow, 기본 압축의 실제 라우팅 완료 |
| 이미지·웹 | PNG/JPEG/GIF/WebP와 승인된 WebSearch/Example Domain WebFetch를 두 조합에서 실제 검증. 최초 실패와 수정 이력 보존 | 최신 후보에서 모든 기능 조합을 다시 통과했다는 주장 |
| 실제 모델 개발 | 실제 모델이 공개 과제 읽기 → 실패 검사 → `parseRetryAfterSeconds` 구현 → 별도 소스 검토 → 독립 28-case oracle 통과. 두 조합 PASS. deadline 과제와 같은 세션의 과제 연속 수행, 효과 후 오류 및 출력 단절 복구 증거도 확보 | 임의 사용자 프로젝트에서의 장기 자율 개발 완료 |
| 실행 관리·원장 | 단일 소유 배타 디렉터리, 실행 전 사용량 예약, 결과 미관측 시 전량 유지, 재조회 시 재실행 방지. 기존 원장 18필드/hash 결속, snapshot 중복 등록 거부, 고정 순서·기한·source hash의 계획 | 실제 누적 원장을 새 관리기에 등록한 end-to-end 성공. 해당 등록은 BLOCKED |
| 과제·산출물·적용 | 검토된 pure-function 과제 등록, 요구·소스·oracle·검토·MCP·events 결속. 완료 후 파일 변조 거부. 변경 제안과 `.tmp/development-target-*` 적용. 두 파일 묶음의 개별 28/32개와 통합 21개 검사 | 일반 프로젝트의 다양한 파일/명령/도구 효과에 대한 완전한 적용·검토·통합 |
| 중단·재개 | 효과 전후 및 출력 단절, 부분 쓰기, 관리기 중단, 첫 복구 관리기의 재중단을 단계별로 대조. 이미 완성된 파일의 중복 쓰기를 막고 남은 효과만 수행 | 전원 손실, PID 재사용, 임의 위치의 연속 중단, 모든 Bash/MCP 부작용의 exactly-once |
| 로컬 배포 | 작업 전용 커밋, 재현되는 ZIP, manifest와 파일 hash 대조, 공백이 있는 새 경로에서 실행. 최신 변경 관련 28파일/1,534 checks PASS | 전체 회귀·실제 backend·장기를 모두 통과한 출하본 |

실제 모델 호출을 하지 않은 것은 아니다. 다음 원자료를 브리핑 과정에서도 다시 확인했다.

| 실제 실행 | luna/max | sol/low | 판정 범위 |
|---|---|---|---|
| 효과 후 오류와 같은 세션 복구 | `native-recovery-0QHFPG/result.json` | `native-recovery-k20rGe/result.json` | 각각 4회 시도, PASS |
| 공개 parser 실제 개발 | `native-development-VnfBni/result.json` | `native-development-eMiIb5/result.json` | 각각 5회 시도, 개발·독립 oracle·cleanup PASS |
| Agent 자식 | `native-headless-151bf2908b20482bbc536414c103f274/result-single.json` | `native-headless-866bbfe6ea1246edbd80926155a22d3a/result-single.json` | 지정 모델/effort, PASS |
| 신규 inline Workflow | `native-headless-46641303ad28491cbf7b7a044524c44f/result-single.json` | `native-headless-b6ba596427674331913aabce19a322a4/result-single.json` | 지정 모델/effort, PASS |

이 표의 상대경로는 구현 worktree의 `.tmp/` 아래다. 실행별 후보와 상세 이력은 [진행 기록](unattended-release-progress.md)에 있다. 최신 실제 backend 검증 후보는 `3903e0a`이며 text/stream-json을 두 조합에서 4회 성공했다. 최신 `6c728af`의 복구 시험은 실제 native에 공개 loopback 응답을 공급한 시험으로, **최신 ZIP의 전체 backend 검증은 아직 아니다.**

실제 호출의 최신 누적 근거는 원장(`../.tmp/unattended-release/live-stream-3903e0a-20260914/result.json`)과 실행 manifest(`../.tmp/unattended-release/live-stream-3903e0a-20260914/manifest.json`)다.

| 실제 원장 항목 | 값과 한계 |
|---|---|
| 누적 시도 | 297~301회. 과거 timeout 구간의 정확한 횟수 불확실성 보존 |
| 관측된 토큰 | input 1,012,355 / output 43,037 |
| 관측된 native 단계 시간 | 2,766,373ms. 전체 작업 시간과 다르며 일부 duration 미관측 |
| 미관측 사용량 | 4회, input 524,288 / output 131,072 예약 유지 |
| 추가 과거 전량 예약 | input 262,144 / output 65,536 유지 |
| 미관측 시간 예약 | 160,000ms 유지 |
| 예약까지 포함한 차감 | attempts 301 / input 1,798,787 / output 239,645 / time 2,926,373ms |
| 현재 시험 상한 | attempts 301 / input 1,973,481 / output 326,903 / time 4,546,653ms |
| 남은 요청 예약 | 0. 요청 상한을 자동 증액하지 않음 |

**301은 담당 agent가 측정에 근거해 정한 이 작업의 누적 시험 상한이다. 구독 계정의 서비스 quota가 소진됐다고 확인한 값이 아니며 추가 결제가 필요하다는 결론도 아니다.** 검색 토큰 미보고와 첫 실패의 사용량 미관측도 남아 있다. 공개 합성 원장에서 쓰는 초기 37/차감 51 등의 숫자는 별도 fixture이며 실제 누적 301을 초기화한 값이 아니다. 마지막 복구 변경과 이번 마감에서 추가 실제 모델 요청·credential 읽기는 0이다.

현재 작업으로 마감한 `6c728af`는 **파일 쓰기를 복구하던 관리기가 다시 중단되어도 원래 증거를 보존하고 이어가는 수정**이다. 6파일에서 242줄 추가/47줄 삭제했다. 첫 복구의 claim·events를 덮어쓰지 않고 별도 resume claim을 사용한다. 원래 소유자와 복구자의 종료, 소스 정체성과 내용 hash, 사건 순서를 대조한 뒤 남은 파일만 쓴다.

최신 새 배포본에서는 `claim/start/first-confirm/last-write/written/done`의 6경계 × 두 조합, 12사례가 같은 세션의 finish와 다음 과제까지 통과했다. 이미 마지막 파일이 완성된 3경계는 그 파일을 다시 쓰지 않았고, 나머지 3경계는 남은 파일 하나만 썼다. 원래 `MANAGER_INTERRUPTED`와 미관측 전량 예약을 유지했다. 완료 재조회 때 새 효과는 0이었다. 이 시험은 공개·검토된 좁은 소스에 대한 것이며 검토되지 않은 실제 모델 소스를 자동 승인하는 기능이 아니다.

12사례는 보수적 예약을 포함해 97,944ms로 정한 900,000ms 안에 끝났다. 부분 적용 거부, 두 파일의 개별 28/32개 및 통합 21개, 변조 거부, 출력 단절 복구를 확인했다. 관련 회귀는 **28파일/1,534 checks, 실패 0, skip 0**이었고 기존 MCP permissionWarnings 4건은 별도로 보존했다. 이것은 해당 변경의 검사 집합이며 프로젝트의 모든 테스트가 통과했다는 뜻은 아니다.

로컬 후보는 Clauduct-6c728afa39bd.zip(`../.tmp/unattended-release/implementation/.tmp/release-artifacts-5e5d8e5035764354bf3c6a9e404f4c4f/Clauduct-6c728afa39bd.zip`)이다. SHA256은 `25a7b3d0cb724b17fb7f2af058165954ad56120f333baae9c70c6933c26d638e`이며 이번 브리핑에서도 파일 hash를 재확인했다. 두 번 만든 ZIP이 같았고, 내용은 205파일/압축 해제 1,961,735bytes다. 새 실행 경로는 `D:\AIDEV\Clauduct\.tmp\unattended-release\p 939fd22d\Clauduct`다. 최종 증거(`../.tmp/unattended-release/p 939fd22d/Clauduct/.tmp/development-recovery-reentry-20260914/artifact-final.json`)는 배포 파일 불변, 실행 소스 40개 hash와 manifest 일치, 149개 소유 확인 범위의 잔여 0, 실제 원장 불변을 기록한다. 그 파일 자체도 `releaseVerdict: HOLD`, `archiveActualBackendVerified: false`, `wholeProjectVerified: false`, `longStageEvidence: false`로 남아 있다.

출하를 막는 일은 다음처럼 구분된다.

| 항목 | 현재 종류 | 완료가 안 되는 구체적 이유 |
|---|---|---|
| 정상 인증 갱신 | 핵심 구현·실증 미완료 | credential supplier의 `force`는 파일 재조회다. 정상 refresh 주체와 제품 연결을 완성하지 않았다. 공개 auth-owner 검사는 있었지만 실제 proxy는 초기화 응답 전에 종료했다. account/read·refresh·model turn은 0이다 |
| 기본 400K/320K 압축 | 필수 실제 시험 미실행 | 기본 설정의 메인·자식 압축과 개발 상태 보존, 압축+재시작+자식 실패의 결합(F10/F18)을 실행하지 못했다. 축소 창·합성 라우팅으로 대체할 수 없다 |
| 일반 프로젝트 무인 개발 | 제품 범위 미완료 | 등록된 좁은 과제와 공개 두 파일 프로젝트까지다. 다양한 실제 파일·Bash/MCP 효과의 조회, 재개, 검토, 적용과 통합이 전체로 연결되지 않았다 |
| 관리기의 실제 누적 원장 연결 | 특정 실행 경로 BLOCKED | 실제 원장 registry 생성이 `ERR_ACCESS_DENIED`로 거부됐다. 공개 원장의 성공을 실제 등록 성공으로 간주할 수 없다 |
| 부모·자식·Workflow | 실패와 기능 공백 | 직접 부모 알림 전달 실패, 실패/취소 자동 복귀, 모델 자식 취소·늦은 응답 격리, Workflow cache-miss/nesting/custom agentType 미완료 |
| 통신·회귀 | 미해결 실패 | 전체 gateway 20초 검사 실패, TCP close/fallback 종료 원인 미확정. cancel의 간헐 timeout과 과거 request-inspector/user-session/native 동시성 실패의 전체 해결 증거가 없다 |
| 설정·내구성·보안 조합 | 필수 미검증 또는 특정 차단 | hooks/plugin/MCP/UI/permission/plan 전체 조합, 실제 cache hit, background 세션 분리, PID 재사용, 기록 잘림·디스크 부족·전원 손실, 악성 입력과 권한/egress 결합의 전체 범위가 남았다 |
| 같은 후보의 전체 검증 | 필수 미완료 | 최신 ZIP의 실제 backend 전체 기능/장애/회귀가 없고, 과거 후보의 개별 PASS가 분산돼 있다. Codex `0.154.0`과 기준 `0.153.4`의 `CLI_VERSION_UNVERIFIED`도 남았다 |
| 실제 요청 한도 | 운영 상한 도달 | 현재 정한 301회 누적 상한에서 새 요청을 예약할 수 없다. 서비스 계정 quota 소진 여부와는 별개다 |
| 장기 단계 | 양쪽 모두 미착수 | 핵심 선행 조건이 닫히지 않았다. 구현 작업 시간을 장기 연속 시험 시간으로 셀 수 없다 |

과거 실패도 없애지 않았다. gateway 전체 검사는 완료된 checks 19 지점에서 20초 상한 실패를 보였고 변경 전 `4ec` 후보에서도 재현됐다. cancel 검사는 15초 간헐 실패 뒤 최근 6회 PASS가 있었지만 최초 원인을 확정한 것은 아니다. 과거 확대 회귀의 request-inspector 84/90, user-session 20초 timeout, native 동시 실행의 `ECONNRESET`도 보존되어 있으며 최신 후보에서 모두 해결됐다는 증거가 없다. 이 문장은 이번 마감에서 그 실패들을 새로 실행했다는 뜻이 아니다.

차단은 해당 효과에만 적용했다. 주요 기록은 실제 원장 registry의 FileSystemWrite 거부, 과거 `f57` 사후검사의 Node Permission Model/process 조회 경계, 동적 링크 시험, worker fsync의 전원 손실 내구성 시험, Workflow cache-miss 경로, 실제 auth-owner proxy 초기화, 일부 공식 문서/런타임 다운로드 접근이다. 권한을 제거하거나 다른 도구·경로·인증 파일 편집으로 같은 거부 효과를 재현하지 않았다. 이 중 일부는 출하 증거 수집을 직접 막고 일부는 선택한 조사 수단의 차단이다. 모든 차단이 전 제품 개발 불가능을 뜻하지 않으며, 실제 독립 로컬 작업은 계속 진행했다. 이번에는 사용자가 작업 중단을 요청했으므로 더 진행하지 않는다.

사용자가 수동 종료한 인증 검사 사건은 **현재 차단 원인이 아니다.** 제가 만든 `auth-owner-public-2AlpMb` 검사에서 JSON escape된 Windows 경로와 자동 소유권 비교가 맞지 않아 회수를 확인하지 못했고, 제공한 수동 스크립트에서도 `IDENTITY_MISMATCH`가 발생했다. 사용자 관리자 실행 후 종료 확인(`../.tmp/unattended-release/implementation/.tmp/auth-owner-public-2AlpMb/manual-stop-final-check.json`)은 원래 PID 12752/11068/23148 인스턴스가 없고 `stopped: true`, 잔여 0임을 기록했다. 사용자에게 부정확한 종료 안내로 추가 부담을 준 것은 검사기·안내의 문제였다. 현재 추가 종료 조치는 요구하지 않는다.

명세의 F01~F23 전체 판정은 아래와 같다. 부분 PASS가 있어도 필수 전체 범위가 닫히지 않은 행은 완료로 표시하지 않았다. 최신 복구와 ZIP 증거를 반영했으므로 진행 기록 상단의 오래된 일부 예시보다 이 표의 마감 판정이 최신이다.

| ID | 마감 판정 | 확보한 부분 증거 / 남은 범위 |
|---|---|---|
| F01 | IN_PROGRESS | 연결 전 DNS 복구와 오류 분류 PASS / TLS 음성 서버 준비 FAIL, 나머지 실제 단절 복구 미완료 |
| F02 | IN_PROGRESS | Retry-After parser·영속 대기·재시작 PASS / 실제 quota 경계 복구 미완료 |
| F03 | IN_PROGRESS | 반복 503 공개 응답, 실제 모델의 주입 오류 후 같은 세션 복구 PASS / 자연 backend 장애·장기 복구 미완료 |
| F04 | IN_PROGRESS | 공개 응답의 HTTP 200 error·미완료 구분과 native 재개 PASS / 실제 backend 결합 미완료 |
| F05 | IN_PROGRESS | 전달 후 단절·UTF-8·순서 오류의 제한된 native 복구 PASS / 실제 backend 결합 미완료 |
| F06 | IN_PROGRESS | 효과 전후·부분 쓰기·관리기와 첫 복구의 재중단 6경계×두 조합 PASS / 일반 효과, 추가 연속 중단, 장기 실제 결합 미완료 |
| F07 | IN_PROGRESS | bounded 출력, 실제 개발의 최종 pipe 단절과 같은 세션 복구 PASS / 장기 관리기 전체 미완료 |
| F08 | IN_PROGRESS | 공개 cache 교체·401 재조회·취소/예산 검사 PASS / 정상 갱신 주체 구현·실제 만료 경계 미완료 |
| F09 | IN_PROGRESS | 공개 401/403/계정 변경/깨진 cache 거부 PASS / 실제 철회·갱신 실패의 개발 복구 미완료 |
| F10 | NOT_RUN | 기본 400K/320K 메인·자식 반복 압축과 후속 개발 |
| F11 | IN_PROGRESS | 배타 소유·중단 후 정산 시 재실행 0 PASS / 실제 backend·동시 소유 전체 미완료 |
| F12 | IN_PROGRESS | 연속 완료 증거 및 메인 조정 relay PASS / 직접 부모 알림 FAIL, 실패·취소 자동 복귀 미완료 |
| F13 | IN_PROGRESS | native Bash background 취소·형제 보존 4사례 PASS / 모델 Agent 자식·늦은 upstream 격리 미완료 |
| F14 | IN_PROGRESS | 긴 journal/transcript의 제한된 판독 PASS / cache-miss 및 실제 긴 기록 재개 미완료 |
| F15 | IN_PROGRESS | 30초 admission 기한·짧은 회복 검사 PASS / 전체 동시 실행 간헐 FAIL 미해결 |
| F16 | IN_PROGRESS | 예약·미관측 보존·변조 거부·실제 단문 정산 증거 / 실제 registry BLOCKED, 잘림·디스크 부족·전원 손실 미검증 |
| F17 | IN_PROGRESS | 해당 실행들의 정상 cleanup와 잔여 0 / 장애·취소·장기의 전체 종료 조합 미완료 |
| F18 | NOT_RUN | 기본 압축·프로세스 재시작·자식 실패 결합 |
| F19 | IN_PROGRESS | 공개 native의 허용 worker 효과 1회·거부 효과 0회 PASS / hooks/MCP/UI 전체 및 실제 모델 결합 미완료 |
| F20 | IN_PROGRESS | 좁은 개발 소스·도구 경계와 독립 oracle 보존 / 전체 native 악성 입력·권한·egress 결합 미완료 |
| F21 | IN_PROGRESS | 실제 main·Agent·신규 Workflow의 두 조합 개별 증거 / 같은 최신 후보의 전체 역할·기본 압축 미완료 |
| F22 | IN_PROGRESS | binding 재사용·변조·경로·정산 조작 거부 / 실제 전체 조합 미완료, 동적 링크 BLOCKED |
| F23 | IN_PROGRESS | 부분 적용·가짜 성공·oracle/정의/계정/경로·검사 수 변조 거부 / 일반 프로젝트·실제 모델 전체 미완료 |

장기 시험은 다음 사건 수까지 요구한다. 단순히 프로세스를 켜 둔 시간을 채우는 시험이 아니다.

| 단계 | 조합별 요구 | 현재 |
|---|---|---|
| 4시간 | 검증 완료 개발 단계 10개 이상, resume 1회 이상, 계획된 장애 복구 1회 이상 | 둘 다 미시작 |
| 24시간 × 3회 | 각 실행 개발 30개 이상. 3회 합계 기본 메인 압축 3회 이상, 자식 압축 1회 이상, 정상 갱신 1회 이상, 재개·취소·형제 실행 | 둘 다 미시작 |
| 72시간 | 개발 100개 이상, 메인 압축 10회 이상, 자식 압축 2회 이상, 정상 갱신 2회 이상, 소유권 보존 재개 및 반복 병렬·취소·복구 | 둘 다 미시작 |

명세대로 **한 조합은 4 + 24×3 + 72 = 148시간(6일 4시간)**이며 두 조합을 순차로 수행하면 **296시간(12일 8시간)**이다. 병렬화의 자원·사용량·간섭 조건이 검증된 계획은 아직 없다. 이 숫자는 시험 실행 시간뿐이고 구현·실패 분석·재시험·인증 경계 대기는 별도다. 따라서 하루 안에 전체 출하를 끝낼 수 있는 명세는 아니었다. 그렇지만 지금 미완료인 이유가 오직 이 최소 시간 때문도 아니다. 아직 장기 시험에 진입하지 못했으며, 선행 핵심 구현보다 세부 fixture를 계속 늘린 우선순위 문제가 함께 있었다.

나중에 사용자가 재개를 요청한다면 먼저 정상 갱신의 구현 경로, 기본 압축을 포함한 실제 개발 입력, 일반 프로젝트와 실제 누적 원장 연결을 어떻게 완성할지 좁혀야 한다. 그 뒤 미해결 기능·회귀를 닫고 같은 후보를 고정하여 장기 시험을 진행해야 한다. 이는 미완료 작업의 의존관계 설명이며 지금 실행 중인 계획이 아니다. 이번 요청에 따른 마감 산출물은 현재 커밋·후보·기존 증거의 보존, 이 브리핑, 전체 변경 목록과 중단 상태 기록이다.
