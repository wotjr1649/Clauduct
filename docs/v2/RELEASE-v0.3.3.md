# Clauduct v0.3.3

Windows x64용 Go V2 유지보수 릴리스 후보다. 배포 여부와 검사 결과는 해당 후보 commit의
출하 검증 기록으로 확인한다.

- 공개 저장소에는 제품 코드·공개 문서·설치 스크립트만 남겼다. 이전 Node 구현(V1), 테스트, 검증
  기록은 빠졌다. 분리 직전의 전체 트리는
  [1b1c5e1](https://github.com/wotjr1649/Clauduct/tree/1b1c5e19b3f33fda63254b2da7c9d0b372553481)에
  고정돼 있다. 공개 CI는 gofmt·vet·build를 본다. 전체 테스트는 로컬에서 돈다
  ([#80](https://github.com/wotjr1649/Clauduct/pull/80)).
- forked Skill(`context: fork`)을 실행한다. 파일 skill과 내장 `/code-review` 모두 해당한다. 모델이 Skill 도구로 부른
  fork의 자식은 native가 그 턴에 기록한 모델·effort로 실행한다(선택 출처 `native-fork`). TUI에서 fork가
  백그라운드로 도는 동안 부모가 빈 턴으로 기다리면, 그 턴을 오류 대신 대기로 처리한다
  ([#81](https://github.com/wotjr1649/Clauduct/issues/81)).
- backend 요청의 10분 전체 상한을 없앴다. 대신 기준선 규칙인 유휴 10분(헤더 뒤 backend 바이트 없음)과
  60분 전체 천장을 둔다. 10분 넘게 흐르는 답이 중간에 잘리지 않는다
  ([#82](https://github.com/wotjr1649/Clauduct/issues/82)).
- backend로 보낸 뒤 실패한 요청은 원인 이름으로 보인다. 이전에는 native의 재시도가 재실행 방지에 막혀
  `NATIVE_REQUEST_REPLAY_BLOCKED`만 보였다. 그런 거부에는 `X-Should-Retry: false`를 붙인다. backend 실패
  이벤트의 code·type·incomplete 사유는 고정 어휘로 줄여 메시지와 요청 기록에 싣는다
  (예: `UPSTREAM_RESPONSE_FAILED upstream_code=server_error`). 목록 밖의 값은 `other`이고, backend 원문은
  싣지 않는다. 브리지 계약 오류 4종도 각자의 이름으로 보인다
  ([#84](https://github.com/wotjr1649/Clauduct/issues/84)).
- 끝난 child turn은 재실행 방지 기록을 돌려준다. 자식이 아주 많은 세션도 16,384개 한도에 닿지 않는다.
  끝난 turn의 요청은 `NATIVE_TURN_ENDED`로 거부한다. 상태의 `nativeEvents.replayKeys`와
  `retiredTurns`가 기록 수를 보인다([#70](https://github.com/wotjr1649/Clauduct/issues/70)).
- rate-limit 헤더의 `NaN`·`Inf`는 형식 오류로 처리한다. 이전에는 그런 값 하나 때문에 세션 계정 전체를 쓰지
  못했다([#83](https://github.com/wotjr1649/Clauduct/issues/83)).
- `--update` 뒤 남는 `clauduct.exe.old`에 대해, 다음 실행이 지운다고 안내한다
  ([#78](https://github.com/wotjr1649/Clauduct/issues/78)). 릴리스 절차는 태그와 버전 상수가 다르거나
  `+dirty` 빌드이면 멈춘다([#74](https://github.com/wotjr1649/Clauduct/issues/74)).

기준 클라이언트 상수는 Claude Code 2.1.280 그대로다. 이 릴리스의 로컬 검사와 실제 backend 확인은
자동 갱신된 2.1.281에서 했다. 공개 옵션의 인자 형태는 2.1.280과 같았다. 진단의 `verified`는
버전 문자열 일치만 뜻하므로 2.1.281에서는 false로 보인다.

출하 구성은 Go 1.27.1, `CGO_ENABLED=0`, `-trimpath`다. `clauduct.exe`,
`clauduct-hook.exe`, `clauduct-dev.exe` 세 파일과 `SHA256SUMS`를 함께 사용한다.
설치와 업데이트는 [패키징 안내](PACKAGING.md)를 따른다.

v0.3.3은 세션 기록 형식을 바꾸지 않는다. 따라서 v0.3.2로 되돌릴 때는 바이너리만 교체한다.
v0.3.0으로 되돌릴 때의 제한은 [v0.3.1 안내](RELEASE-v0.3.1.md)와 같다.

Windows 전용이며 서명하지 않는다. 지원 기능과 남은 조건은 [호환성 문서](COMPATIBILITY.md)를
참조한다.

## 출하 검사 (2026-09-24)

태그 `v0.3.3`은 `3ecbc4a`를 가리키고, [Release](https://github.com/wotjr1649/Clauduct/releases/tag/v0.3.3)에 자산 6개가 있다.
검증 기록은 v0.3.3부터 공개 저장소에 두지 않으므로 결과만 적는다.

- 태그의 깨끗한 checkout 두 곳에서 빌드했고, 두 번째는 별도 build cache를 썼다. 세 바이너리는 바이트 단위로 같았다.
  `clauduct-dev version`은 `0.3.3`, commit `3ecbc4a9bdf7134e36b1e0da543a885e2af1fb11`이고 `+dirty`는 없다.
- 격리 설치로 새 설치, v0.3.2 발행 자산 설치, v0.3.3으로 업데이트, v0.3.2로 되돌리기를 차례로 했다. 모든 단계에서
  digest와 버전 스탬프가 맞았고 `.old` 파일과 PATH 변화는 없었다.
- 설치한 v0.3.3으로 실제 backend 세션을 돌렸다(luna/low, 시도 5회). 입력, `/clear`, 백그라운드 자식 위임을 보냈고
  거부·끊김·실패는 0이었다. hook이 설치됐고 자식 보고가 도착했으며, 끝난 자식 turn 하나가 재실행 방지 기록을 돌려줬다.
- 발행 후 받은 자산 6개는 빌드한 파일과 같았고, Release API의 digest는 `SHA256SUMS`와 같았다.
  `install.ps1 -Tag v0.3.3`으로 GitHub에서 설치했다. 격리된 v0.3.2의 `clauduct --update --yes`는 v0.3.3으로 교체했고,
  남은 `.old`는 다음 실행에서 사라졌다. 두 번째 `--update`는 `already current`였다.
