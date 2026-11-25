# v0.6.17 — 선택형 격리 모드와 세션 재개·설정 개선

선택형 격리 모드를 추가하고, 기본 모드의 background 세션 재개와 계정 모델 목록 캐시, Clauduct 설정 파일 처리를 고쳤다.

## 바뀐 것

- **기본 모드에서 `stop`·`rm` 직후 같은 세션을 `--resume`으로 다시 열 수 있다.** 관리 명령이 세션 잠금 해제를 기다려 직후 재개가 잠금에 걸리는 문제를 고쳤다. [#399](https://github.com/wotjr1649/Clauduct/issues/399), [PR #400](https://github.com/wotjr1649/Clauduct/pull/400).
- **쓸 수 없는 계정 모델 목록은 마지막 정상 캐시를 덮지 않는다.** 받아 온 목록에 이 빌드가 라우팅할 수 있는 모델이 있는지 확인한 뒤 저장한다. [#401](https://github.com/wotjr1649/Clauduct/issues/401), [PR #404](https://github.com/wotjr1649/Clauduct/pull/404).
- **v0.6.17 설치 스크립트·업데이트·일반 실행은 `~/.clauduct/settings.json`을 만들거나 바꾸지 않는다.** 파일이 없으면 공장값을 쓰고, v1 파일에서 생략한 최상위 키는 메모리에서 채운다. 이전 updater가 호출하는 `--dev --sync-settings`는 안내만 출력하고 성공한다. 편집할 기본 파일은 `--dev --init-settings`로 만들 수 있다. [설정 동기화](SETTINGS.md#설정-동기화-v0617), [#402](https://github.com/wotjr1649/Clauduct/issues/402), [PR #406](https://github.com/wotjr1649/Clauduct/pull/406).
- **`isolation: "on"`으로 전용 native 설정 폴더와 supervisor를 쓸 수 있다.** 전용 `CLAUDE_CONFIG_DIR`에는 저장된 Claude 로그인을 옮기지 않는다. native 2.1.331의 print·TUI·background 계측 표본에서 telemetry, eval/feature flag, bootstrap, oauth/profile 요청이 각각 0건이었다. 기본값은 `off`다. [격리 모드 설정](SETTINGS.md#격리-모드-v0614), [격리 모드 계측](COMPATIBILITY.md#서버-경계-원장--격리-모드-v0614), [#379](https://github.com/wotjr1649/Clauduct/issues/379), [PR #405](https://github.com/wotjr1649/Clauduct/pull/405).

## 출하 검사

태그·commit, 후보 `clauduct.exe`와 `clauduct-probe.exe`, native 버전과 SHA-256에 결속한 `GATE-T1` 결과는 발행 뒤 이 문서에 기록한다.

## 출하 신원

| 항목 | 값 |
|---|---|
| 태그 | 발행 후 기록 |
| `clauduct.exe` | 발행 후 기록 |
| `clauduct-probe.exe` | 발행 후 기록 |
| `install.ps1` | 발행 후 기록 |
| `uninstall.ps1` | 발행 후 기록 |
| `SHA256SUMS` | 발행 후 기록 |

## 알려진 제한

- **격리 모드도 사용자 자산과 대화 이력을 공유한다.** 일부 폴더는 Windows junction으로, `history.jsonl`은 hard link로 공유한다. 이 방식은 native 공식 문서에 명시되어 있지 않다. native 2.1.331의 `claude project purge`가 `history.jsonl`을 교체해 링크가 끊어지는 것을 확인했다. Clauduct는 다음 격리 실행에서 링크 단절을 알리며 두 이력을 자동으로 합치지 않는다. [공유 자산과 이력](SETTINGS.md#격리-모드-v0614).
- **격리 세션에서 OAuth가 필요한 원격 MCP 서버는 `/mcp`에서 서버별로 한 번 인증한다.** 미러링되는 MCP 정의나 설정에 평문 비밀값을 넣으면 전용 폴더에도 복제된다. 격리 세션 안에서 바꾼 파생 설정은 다음 실행에서 덮일 수 있다. 실행 중인 다른 모드의 job은 `--resume`으로 열 수 없다. [격리 모드 설정](SETTINGS.md#격리-모드-v0614).
- **격리 모드에서도 외부 통신은 남는다.** native 2.1.331의 격리 표본에서 공식 플러그인 버전 조회와 git의 GitHub `CONNECT` 등 Anthropic 외부로 나가는 통신을 관측했다. WebFetch 사전 domain 점검과 공식 marketplace 자동 등록은 격리 설정만으로 끄지 않는다. [격리 모드 서버 경계](COMPATIBILITY.md#서버-경계-원장--격리-모드-v0614).
- **파일 기반 Workflow(`named`·`scriptPath`)는 검증한 native 2.1.325 이미지에서만 허용한다.** native 2.1.331도 재검증에서 Read 금지를 지키지 못했다. 두 모드 모두 이 이미지의 파일 소스를 `CLAUDUCT_WORKFLOW_SOURCE_IMAGE_UNVERIFIED`로 거부한다. inline `script`는 계속 지원한다. [#403 재검증](https://github.com/wotjr1649/Clauduct/issues/403#issuecomment-7142918183), [호환성 문서](COMPATIBILITY.md).
- 공통 제한은 [알려진 제한 원장](COMPATIBILITY.md#알려진-제한-원장-v0617)에 있다.
