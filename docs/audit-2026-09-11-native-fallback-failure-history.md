# Native fallback 및 제한된 실패 보존

## 근거와 원인 범위

사용자 세션 c4c222f8-9676-4d22-a7e8-72340225b868 종료 JSON은 총 49건/성공 44건/실패 5건(prepare 2, upstream 3)을 기록했다. 최근 목록에서 요청 57은 UNSUPPORTED_EVENT other/identifier, 이어진 58은 REQUEST_STREAM_FALSE였다. 요청 59는 성공했다. 실패 3건의 상세는 최근 16개 범위 밖이었다. cleanup 9개 true 및 종료 SUCCESS는 요청 성공 판정과 다르다.

2026-09-11 읽기 전용 조사에서 설치된 C:/Users/JS/.local/bin/claude.exe(220051616 bytes)의 내장 코드에 다음 경로가 존재함을 확인했다. native 실행·인증 조회·바이너리 수정은 하지 않았다.

- 스트리밍 catch 경로가 CLAUDE_CODE_DISABLE_NONSTREAMING_FALLBACK와 native 내부 feature flag를 평가한다.
- fallback 비활성 분기는 원래 오류를 throw한다. 활성 분기는 onStreamFallback/onStreamingFallback을 거쳐 비스트리밍 함수로 진입한다.
- 비스트리밍도 실패하면 별도의 오류 처리로 넘어가며 마지막 오류가 사용자에게 표시될 수 있다.
- 따라서 기존 MAX_RETRIES=0만으로 이 별도 전환을 막는다고 볼 수 없다.

[공식 환경 변수 문서](https://code.claude.com/docs/en/env-vars)는 CLAUDE_CODE_DISABLE_NONSTREAMING_FALLBACK=1이 스트리밍 실패 후 비스트리밍 전환을 막고 오류를 retry 계층으로 전달한다고 설명한다. 기존 재시도 차단 설정은 유지한다. GitHub 사용자 제보는 결론의 근거로 사용하지 않았다. 로컬 native 버전 문자열은 이번 조사에서 실행해 조회하지 않았고, 제공 JSON의 0.154.0은 Codex clientVersion이므로 Claude 버전으로 사용하지 않는다.

이 경로와 요청 순서는 fallback 해석을 뒷받침한다. 다만 해당 세션의 native debug 디렉터리가 없어 실제 분기 실행을 직접 확인한 것은 아니다. 최초 미지원 이벤트명, account 변경 영향, 모든 오류의 원인은 여전히 미확인이다.

## 묶음 보완

1. 자식 실행의 settings.env와 실제 child env에 공식 fallback 차단 값을 적용한다. 부모 환경을 수정하지 않으며 부모에서 0을 설정해도 해당 child에만 1을 적용한다. 오류를 성공으로 바꾸거나 upstream 재전송/stream 강제 변환을 하지 않는다.
2. gateway에 완료된 실패 최초 8개+최근 8개를 최대 16개 보존한다. 기존 recentRequests 16개와 독립이며 기존 고정 진단 정보만 복사한다. 완료 후 저장하므로 진행 중 요청이 recentRequests에서 밀려도 실패를 보존한다. 요청 시작 순서가 아닌 완료 순서다.
3. request-status의 기존 row sanitizer를 공유해 failureHistory.records와 omitted를 제공한다. 임의 원문/라벨은 추가 공개하지 않는다. 이전 형식은 failureHistory=null, 새 빈 이력은 records=[]다. projection 재적용 시 실패 이력을 유지한다.
4. requestOutcome으로 요청 실패와 프로세스 종료 성공을 분리한다. childExecution 설정 전달은 clientExecutionPolicy의 고정 boolean으로 진단한다. 환경 전달 증거이지 native 런타임 실행 증명이 아니다.
5. 늘어난 유한 진단 레코드에 맞춰 상태 조회 응답 상한을 128→256 KiB로 조정한다. 상한 초과 응답은 계속 거부한다. 파일 저장·원문 요청 복사·계정/토큰 조회·신규 의존성은 없다.

## 통합 검증

Invoke-ClauductNodeTests, 60초 제한, Node.js v24.19.0에서 다음 src 테스트 11개 파일을 함께 실행해 모두 통과했다: native-gateway, launcher-native, native-protocol, native-transport, native, request-diagnostics, upstream-failures, unsupported-event-diagnostics, cancel-snapshot, client-version, compact-policy (파일명은 test- 접두사와 .mjs 접미사).

gateway 46개 검사는 실패 24개 초과 시 최초/최근 유지와 omitted=8, 성공 17개 이후 최초 실패 유지, 진행 중 요청이 성공 18개에 밀린 후 실패해도 보존, snapshot 변경으로 원본 훼손 불가, 구형/악성 진단 projection, 실제 loopback 상태 조회, 256 KiB 초과 거부를 포함한다. launcher는 parent env 불변·child env/settings 동시 적용, cancel-snapshot은 종료 JSON까지 설정 증거를 검증했다. 기존 native 45개(1000 요청/20 동시 agent 포함), request-diagnostics 69개, upstream-failures 84개, unsupported-event-diagnostics 29개 검사도 통과했다.

Verified는 로컬 코드 및 synthetic/loopback 검사다. 실제 native fallback 차단, 최초 UNSUPPORTED_EVENT 해결, SDD 최종 커밋은 Not verified다. 이미 리뷰 완료한 run-02 산출물과 SDD_VERIFICATION.md는 변경·커밋하지 않았다. 기존 session-22는 기록 파일 부재를 전제하므로 재실행하지 않는다. 후속 실사용은 새 프로세스에서 보존된 결과의 통합 단계만 별도 범위로 진행해야 하며, 같은 SDD 전체 시험을 반복해 진단을 얻지 않는다.
