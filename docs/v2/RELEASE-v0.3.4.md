# Clauduct v0.3.4

Windows x64용 Go V2 릴리스 후보다. GPT-6 Sol과 Luna를 라우팅하고 GPT-5.6 Sol과 Luna를 은퇴시킨다.
배포 여부와 검사 결과는 해당 후보 commit의 출하 검증 기록으로 확인한다.

- **모델 표.** fable→`gpt-6-astra`, opus→`gpt-6-sol`, sonnet→`gpt-5.6-terra`, haiku→`gpt-6-luna`.
  기본 effort는 계열 값을 유지한다: astra medium, sol xhigh, terra high, luna max. 내장 역할 Explore와
  general-purpose는 haiku를 따라 `gpt-6-luna`/max로 간다(Plan은 `gpt-6-astra`/medium 그대로).
  두 새 모델은 표에 넣기 전에 실제 backend에서 측정했다. low..max 수용, 도구·reasoning 왕복·이미지 동작,
  텍스트 요청에서 로컬 토큰 계수와 backend 계수 일치를 확인했다([#104](https://github.com/wotjr1649/Clauduct/issues/104)).
- **은퇴.** `gpt-5.6-sol`·`gpt-5.6-luna`와 v0.3.3의 effort별 에이전트 이름(`clauduct-sol-high` 등)은
  `MODEL_RETIRED`로 거부하고 대체 모델을 안내한다. 요청, Agent 호출, 이전 세션의 선택·문맥 기록 어디서
  와도 같다. 새 모델로 자동 대체하지 않는다.
- **위임 메뉴 5종.** `clauduct-astra`·`clauduct-sol`·`clauduct-terra`·`clauduct-luna`·`clauduct-inherit`.
  effort는 Agent 호출의 `effort` 인자로 주고, 없으면 모델 기본값이다. 요청 하나당 약 975토큰이 줄었다
  ([#105](https://github.com/wotjr1649/Clauduct/issues/105)).
- **`--model`만 줄 때.** `clauduct --model sol`은 이제 시작 effort(low)가 아니라 그 모델의 기본 effort(xhigh)로
  돈다. 사용자의 `--effort`는 여전히 이긴다([#87](https://github.com/wotjr1649/Clauduct/issues/87)).
- **모델 표는 한 곳.** 모델별 effort 집합을 두고 모든 사본을 표에서 만든다. Codex 카탈로그의 `ultra`는
  세 모델 모두 backend가 거부해 노출하지 않는다([#103](https://github.com/wotjr1649/Clauduct/issues/103),
  [#106](https://github.com/wotjr1649/Clauduct/issues/106)).
- **새 모델 감지와 측정.** `clauduct-dev doctor`가 Codex 모델 캐시(`~/.codex/models_cache.json`, 모델 필드만)와
  표의 차이를 보고한다: 라우팅하지 않는 모델, effort 차이, 숨겨지거나 사라진 모델과 후속 모델·은퇴일,
  문맥 창, 캐시와 설치 codex의 버전. `clauduct-dev probe accept <model>`은 경로마다 상한을 둔 모델 수용
  점검이다([#107](https://github.com/wotjr1649/Clauduct/issues/107), [#108](https://github.com/wotjr1649/Clauduct/issues/108)).
- **재실행 방지 순회.** 끝난 자식 turn을 정리하는 순회가 열린 자식 turn만 본다. 자식이 많은 세션에서
  요청마다 드는 비용이 세션 길이에 따라 늘지 않는다([#109](https://github.com/wotjr1649/Clauduct/issues/109)).

기준 클라이언트 상수는 Claude Code 2.1.281이다. 2.1.281의 `--agents`는 `--print`와 함께 JSON 파일 경로도
받으며, 역할 탐색이 그 파일을 세션 시작 때 한 번 읽는다.

출하 구성은 Go 1.27.1, `CGO_ENABLED=0`, `-trimpath`다. `clauduct.exe`,
`clauduct-hook.exe`, `clauduct-dev.exe` 세 파일과 `SHA256SUMS`를 함께 사용한다.
설치와 업데이트는 [패키징 안내](PACKAGING.md)를 따른다.

**되돌리기 제한.** v0.3.3으로 바이너리를 되돌리면:

- v0.3.3은 `gpt-6-sol`·`gpt-6-luna`를 모르므로 그 모델로 만든 선택 기록을 거부한다. 그 자식은 재개되지 않는다.
- 반대로 v0.3.3에서 `gpt-5.6-sol`·`gpt-5.6-luna`로 돌던 세션과 자식은 v0.3.4에서 재개되지 않는다. 새 세션을 연다.
- v0.3.3에서 시작한 Workflow는 v0.3.4에서 복구되지 않는다(래퍼 카탈로그 형식이 바뀌었다). 거부로 끝나며
  재실행하지 않는다.

Windows 전용이며 서명하지 않는다. 지원 기능과 남은 조건은 [호환성 문서](COMPATIBILITY.md)를
참조한다.

## 출하 검사 (2026-09-24)

태그 `v0.3.4`는 `be7c689`를 가리키고, [Release](https://github.com/wotjr1649/Clauduct/releases/tag/v0.3.4)에 자산 6개가 있다.
검증 기록은 공개 저장소에 두지 않으므로 결과만 적는다.

- 태그의 깨끗한 checkout 두 곳에서 빌드했고, 두 번째는 별도 build cache를 썼다. 세 바이너리는 바이트 단위로 같았다.
  `clauduct-dev version`은 `0.3.4`, commit `be7c68963fdcfd5ed0f276f15b4ea5d15886fc62`이고 `+dirty`는 없다.
- 격리 설치로 새 설치, v0.3.3 발행 자산 설치, v0.3.4로 업데이트, v0.3.3으로 되돌리기를 차례로 했다. 모든 단계에서
  digest와 버전 스탬프가 맞았고 `.old` 파일과 PATH 변화는 없었다.
- 설치한 v0.3.4로 실제 backend 세션을 돌렸다(시도 5회). 부모는 `--model gpt-6-luna`만 주어 기본 effort max로 돌았고,
  자식은 `clauduct-luna`에 `effort: low` 인자로 돌았다. 입력, `/clear`, 백그라운드 자식 위임을 보냈고 거부·끊김·실패는 0이었다.
  자식 보고가 도착했고, 끝난 자식 turn 하나가 재실행 방지 기록을 돌려줬다. 첫 실행은 모델의 Agent 호출 하나가 native 필수 인자
  없이 나가 거부된 것을 검사 스크립트가 시도로 세어 세션을 일찍 끊었다(제품 결함 아님). 스크립트를 고쳐 다시 돌렸다.
- 발행 후 받은 자산 6개는 빌드한 파일과 같았고, Release API의 digest는 `SHA256SUMS`와 같았다.
  `install.ps1 -Tag v0.3.4`로 GitHub에서 설치했다. 격리된 v0.3.3의 `clauduct --update --yes`는 v0.3.4로 교체했고,
  남은 `.old`는 다음 실행에서 사라졌다. 두 번째 `--update`는 `already current`였다.
