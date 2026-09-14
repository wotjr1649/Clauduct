# 변경된 출하 목표와 작업 판정

2026-09-14 사용자가 정상 인증 갱신·기본 압축 실호출·장기 시험을 제외하고 남은 작업의 구현과 로컬 출하를 요청했다. 목표는 약속된 비-Anthropic 서버 전용 기능과 제한된 실제 개발·복구를 완성하고 동일 최종 후보의 증거로 로컬 출하 PASS를 판정하는 것이다. 외부 게시·배포는 포함하지 않는다.

정상 갱신 판단은 사용자 Codex CLI 로그인 기준, 기본 400K/320K 압축 발동은 사용자 실사용 검증, 4h/24h×3/72h 단계는 범위밖이다. 기존 설정·변환·라우팅·오류·권한 검사는 유지하며, 제외를 PASS로 기록하지 않는다. 원래 명세와 F01~F23 중 제외된 부분 이외의 필수 요구는 유지한다.

기준 후보는 `6c728afa39bd5d5166dd82ba981d6b366fe03580`, 구현 위치는 `.tmp/unattended-release/implementation`, 전용 branch는 `work/unattended-release-2026-09-13`이다. 사용자 루트의 README +2/-0와 기존 untracked 상태를 보존한다. 수정은 이해한 작업 diff만으로 되돌릴 수 있게 분리한다.

| 순서 | 남은 범위 | 현재 상태 | 필요한 증거 |
|---|---|---|---|
| 1 | gateway·HTTP 종료·취소·동시 실행 | 조사 중 | 기존 실패를 설명하는 작은 재현, 수정 전후 대조, 관련 회귀 |
| 2 | 부모·자식 알림/실패/취소, Workflow 적용 가능한 계약 | 미완료 | 실제 기능과 native 제공 경계 대조, 수정 및 필요한 두 조합 실행 |
| 3 | 실제 개발·소유/사용량·효과 대조·파일 적용·복구 | 부분 검증 | 현재 실제 경로의 좁은 개발·독립 oracle·중복/유실/거짓 완료 0 |
| 4 | 남은 설정·도구·권한·변조·기록 경계 | 부분 검증/특정 차단 | 요구별 정상·거부 증거. 기존 guard 거부는 우회하지 않음 |
| 5 | 동일 최종 후보 회귀·실제 실행·로컬 ZIP | 미완료 | 관련/전체 회귀, 필요한 실제 luna/max·sol/low, 재현 ZIP/hash/새 경로/회수 |

장기 시험 전용 반복과 관리기 확장을 추가하지 않는다. 기존 통과 증거는 코드 영향과 적용 범위를 대조하여 재사용하고, 새로운 실패·변경·미검증이 필요한 검사만 실행한다. ZIP은 최종 후보가 정해진 뒤 만든다.

첫 조사 한도는 외부 요청 0, credential 읽기 0, 공개 loopback만, 검사당 20초 이하와 전체 로컬 runner 60초 이하다. 기존 실패의 동일 반복·시간 상한 확대는 하지 않는다. 실제 호출은 최신 `live-stream-3903e0a-20260914`의 누적 시도 297~301 및 미관측 예약을 이어받는다. 현재 요청 cap 301/잔여 0이므로 새 실호출 전에 변경된 제한 실행 목록과 기존 측정으로 별도의 유한 예산을 산정·기록해야 하며, 아직 새 요청은 예약하지 않았다. 사용자 비용 축소 요구를 반영하고 과거 소비를 초기화하지 않는다.

구현 영향 회귀는 30초 runner 안에서 수행했고, 새 native 비교는 실행 전 별도 budget.json에 각 30초/16개 공개 응답 상한을 기록했다. 장기 시험으로 확대하지 않았다.

목표 도구의 기존 목표는 blocked이며 새 objective 등록은 `unfinished goal` 때문에 거부됐다. 완료되지 않은 예전 목표를 완료로 변경하지 않았다. 이 문서가 사용자의 최신 범위를 반영한 현재 로컬 목표다. 사용자 재개 요청으로 개발 권한 차단은 해소되었고, 기존 개별 guard 차단은 유지한다. 현재 출하 판정은 HOLD다.

## 재개 후 관측과 구현

- HTTP 종료 비교: 기존 경로 3회 중 2회는 약 1초 fallback, FIN을 보내는 실험 경로는 3회 모두 약 4~13ms였다. 그러나 해당 수정의 실제 기존 검사에서 registration ECONNRESET과 반닫기 응답 유실이 발생했다. 실험 수정을 철회했으며 `src/http-close.mjs`는 기준 코드와 같다. 증거는 `.tmp/release-completion-20260914/close-differential-result.json`, `http-gateway-first-fix.json`이다. 지연 상한이나 assertion을 완화하지 않았다.
- 실패 알림: 기존 구현은 실패 알림을 무조건 거부했다. 새 `failed()`는 현재 검증 요청의 실패만 기록하며 이후 begin/정상 완료/늦은 callback과 구분한다. 부모 재개는 실패 receipt, 직접 자식 관계, 같은 metadata, 새 native API 오류의 ID/role/stop_sequence/시간을 함께 요구한다. 기존 성공 receipt와 섞인 batch를 원자적으로 소비하며 재사용·위조·동시 재개를 거부한다. 취소·사용자 중단은 실패 성공으로 대체하지 않는다.
- 최초 실패 baseline은 `failed-completion-baseline.json`의 NOTIFICATION_HEADER다. 최종 `failed-completion-final-regression.json`은5파일 PASS/12616.9509ms이며 새23개, 기존 completion46/batch11/diagnostics69 및 agent-selection 검사를 포함한다. 내부 symlink NOT_RUN은 별도로 유지한다. 실제 HTTP handler의 실패 receipt 연결과 cleanup도 검사했다.
- 실제 native 비교는 외부 transport 없이 고정 공개 응답만 사용했다. 처음 두 sol fixture는 실행 도구 목록에 Read가 없어 자식이 시작되지 못했고, 첫 fixture의 main 조기 오류도 보존했다. 구성 오류를 수정한 sol root `native-failure-44ac0e0036f24efba77b3bfa42f6b826`과 luna root `native-failure-394d0c8e9e9d45488f28f734fb809ead`에서 각각 자식 API 실패가 발생했다. sol은7개 로컬 요청/2084ms, luna는2122ms, 두 실행 모두 회수 잔여0이다. 부모 직접 알림/재개는 실패로 남았다. native 오류 record의 관측 형식은 `native-failure-record-shape.json`에 값 없는 구조로 남겼다.
- `.tmp/release-completion-20260914` 아래 증거와 새 native fixture만 추가했다. 개인 profile·기존 세션 원문·인증 값을 입력으로 복제하지 않았고 실제 backend 추가 요청0이다. 단위 검사, native+공개 응답, 실제 backend를 구분한다.

## 확인한 native 경계와 후속 범위

[공식 subagent 문서](https://code.claude.com/docs/en/sub-agents#resume-subagents)는 SendMessage 재개의 결과를 중간 부모에게 돌려주는 설명을 interactive 세션에 한정한다. 이번 `-p` 실행에서는 부모 transcript에 알림이 없었고 메인으로 전달되는 것을 직접 관측했다. 문서만으로 모든 버전의 비대화형 동작을 단정하지 않는다. [공식 저장소의 보고 #81438](https://github.com/anthropics/claude-code/issues/81438)에도 중첩 알림의 다른 버전/상황에서의 전달 문제가 있지만, 이는 이번 원인 확정이나 제품 수정 완료의 증거는 아니다.

[공식 Workflow 문서](https://code.claude.com/docs/en/workflows#resume-after-a-pause)는 같은 세션에서 저장 결과를 재사용하고, 실패 지점과 그 이후를 재실행하며, 저장 결과 자체가 없으면 `nothing to resume`로 종료한다고 설명한다. 따라서 cache miss 용어를 저장 결과 전체 소실과 개별 agent 재실행으로 구분할 필요가 있다. 기존 scriptPath 거부 경로를 새 문서만으로 재시도하지 않았다. Workflow의 현재 미완료는 아직 닫지 않았다.

`CLI_VERSION_UNVERIFIED`는 `docs/audit-2026-09-10-cli-version-policy.md`와 실행 코드에 명시된 비차단 진단이다. 이 문자열 자체를 출하 실패로 세던 브리핑 해석은 정정하며, 실제 호환성 검증 범위는 그대로 필요하다.

다음 구분을 위해 사용자 일반 PowerShell에서 기존 공개 `.NET` TCP probe의 비교 결과를 요청했다. 에이전트 실행의 반닫기 응답 유실이 사용자 터미널에도 적용되는지 확인하기 위한 요청이며, 인증·전역 설정 변경이나 프로세스 종료를 요구하지 않는다. 결과가 오기 전 그 환경의 성공을 가정하지 않는다. 나머지 기능·실제 개발 경로·기존 실제 원장 registry 차단·최종 후보 검증도 미완료다.
