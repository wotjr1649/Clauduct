# Codex CLI 버전 정책 분리

## 변경 범위

사용자 승인에 따라 정확한 버전 문자열 일치만으로 시작을 차단하던 정책을 분리했다. `0.153.4`는 REFERENCE_CLIENT_VERSION(기존 증거 기준)이며 설치 버전 pin이 아니다. 조회된 버전이 유효하면 그대로 전달하고, 기준과 다르면 `unverified`로 표시하며 비차단 경고를 출력한다. 버전이 같거나 다르다는 사실을 실제 프로토콜 호환성의 증명으로 쓰지 않는다.

`src/client-version.mjs`는 길이·숫자 버전 형식·제한된 prerelease/build 표기만 검증한다. 공백·개행·임의 헤더 문자열·누락된 버전은 거부한다. 알려진 비호환 버전 범위가 확인된 것은 없어 임의의 min/max 호환 범위는 만들지 않았다.

`openUserTransport`는 고정된 Codex 실행 파일의 `--version`을 기존 5초/4096-byte 제한으로 한 번 조회한다. 검증 후 반환한 실제 버전을 native transport와 기존 PoC transport에 전달한다. sender는 생성 시점 버전을 보존하고 모든 요청의 `Version`·`User-Agent`에 사용한다. 실사용 factory에는 버전 기본값이 없으며 synthetic loopback factory만 기준 버전을 기본으로 사용한다.

시작 실패 구분:

- CLI_VERSION_UNAVAILABLE: 실행 실패, nonzero exit, signal 종료, 조회 결과 부재.
- CLI_VERSION_INVALID: 조회 형식/길이/버전 문자열이 비정상.
- CLI_VERSION_UNVERIFIED: 기준과 다른 유효한 버전의 경고. 종료 오류가 아니며 실행을 계속한다.

transport diagnostics, request-status, PoC entrySummary에 clientVersion/referenceClientVersion/clientVersionStatus를 전달한다. 관측되지 않은 버전은 null/not-observed이며 기준 버전을 실제 관측값처럼 채우지 않는다. 상태 출력에는 버전 형식 검사를 다시 적용한다. 기존 인증·runtime·계정·요청/응답 프로토콜·재시도·취소 검사는 변경하지 않았다.

`verification/manual-http-probe.mjs`의 독립적인 과거 수동 probe 기준은 이번 launcher 변경의 대상이 아니며 유지했다. 실제 설치 업데이트·다운그레이드·전역 설정 변경·인증 파일 쓰기는 하지 않았다.

## 관측한 검증

- RED: 새 버전 `0.154.0`을 반환하도록 하는 회귀 검사가 수정 전 CLI_VERSION_CHANGED로 실패했다.
- `src/test-client-version.mjs`: 통과. 기준 버전, 새 release, prerelease의 native/PoC 두 transport에서 12개 실제 loopback HTTP 요청을 보냈다. Version/User-Agent·diagnostics·request-status·summary와 생성 후 옵션 변경에도 버전이 유지되는지 확인했다. malformed/version injection/query 실패/미관측 진단도 확인했다.
- `src/test-native-transport.mjs`: 통과.
- `poc/test-gateway.mjs`: 74 passed / 0 failed.
- `poc/test-user-session.mjs`: 89 passed / 0 failed. 인증 캐시·환경·시작 조건의 기존 합성 검증 포함.
- `src/test-native-gateway.mjs`: 43 passed. 비정상 버전 진단 제거 포함.
- 현재 설치된 codex.exe --version의 실제 출력은 codex-cli 0.154.0이었다. 그 출력을 변경된 parser/policy에 전달해 clientVersion=0.154.0, referenceClientVersion=0.153.4, clientVersionStatus=unverified를 확인했다. 인증 경로는 실행하지 않았다.

테스트는 외부 요청·실제 인증 읽기 없이 loopback과 합성 데이터로 수행했다. 실사용 transport 생성에 유효 버전을 넣는 검사를 한 번 시도했으나, 제한된 테스트 프로세스의 execArgv를 기존 checkRuntime이 DEBUG_RUNTIME_UNSUPPORTED로 거부했다. 보호 장치를 해제하거나 제한 플래그를 제거해 재시도하지 않았으며, 유효 실사용 factory 생성/인증 요청은 미검증으로 남긴다. 버전 형식 거부와 실제 wire 전달은 각각 validator와 동일 sender의 loopback으로 검증했다.

## session-18 인계

새 Clauduct 프로세스에서 session-18 프롬프트를 진행할 수 있도록 로컬 변경을 준비했다. 기준 버전 차이 경고는 예상 동작이다. 실제 인증된 native 시작 및 모델 요청 호환성은 사용자의 다음 실행 증거로 확인해야 하며, 버전 차단 제거로 기존 UNSUPPORTED_REQUEST가 해결됐다고 주장하지 않는다.

run-02 HEAD는 298a533이고 보존된 untracked test/untag.test.mjs는 변경하지 않았다. SHA256 CD24CD98AAB6746F490BAA168601FE6E17DC35914CB3B9D24F662744BE0921F3을 확인했다. 기존 session-18의 Bash 파일 실행·RED 이어받기·비동기 SDD·실패 진단 절차는 유지한다.
