# v0.6.10 — 추론 요약 화면 표시, 분류기 replay 오인 차단 수정

2026-10-04. 범위는 [#284](https://github.com/wotjr1649/Clauduct/issues/284)이다. 남은 제한과 다시 볼 조건은
[COMPATIBILITY.md](COMPATIBILITY.md)의 3절 "알려진 제한 원장 (v0.6.10)"이 관리한다.

## 바뀐 것

- **backend 추론 요약을 화면에만 표시**([#277](https://github.com/wotjr1649/Clauduct/issues/277), PR #285 /
  [#286](https://github.com/wotjr1649/Clauduct/issues/286), PR #287)
  - `showThinkingSummaries`를 켠 TUI 세션(`thinking.display: summarized`)의 main turn 요청에만 backend에
    `reasoning.summary`(auto)를 요청한다. 받은 요약은 세션 plugin이 `clauduct-native-events: ∴ …` 한 줄로 답보다 먼저 보인다.
  - 이 줄은 모델 요청과 대화 메시지에 실리지 않는다(`--resume` 뒤 포함). 세션 기록에는 `system/informational` 행으로 남는다.
    thinking 블록과 서명은 만들지 않는다.
  - 요약은 backend 출력이라 제어·서식 문자와 `**`를 지우고 최대 8줄 × 240자로 자른다. 다음 turn에 돌려주는 reasoning
    기록(`redacted_thinking`)에는 요약을 넣지 않는다.
  - 요약을 요청하면 첫 text가 늦어진다(같은 입력 arm당 10회, luna +2.7초·sol +0.9초, input·cache 적중 변화 없음). 그래서 TUI
    기본값 `updates`, -p·SDK, subagent·teammate·workflow agent, 보조 요청, compaction, 토큰 계수 요청은 요약을 요청하지 않고
    요청 본문이 v0.6.9와 같다.
- **auto 모드 분류기 요청의 replay 오인 차단 수정**([#289](https://github.com/wotjr1649/Clauduct/issues/289), PR #291)
  - native는 형제 agent가 거의 동시에 같은 동작을 하면 같은 바이트로 분류를 묻는다. v0.6.5부터 gateway가 두 번째 질문을
    재전송으로 보고 `NATIVE_REQUEST_REPLAY_BLOCKED`로 막았다. 판정을 받지 못한 호출은 native가 실행하지 않는다(v0.6.4 측정).
    재현한 경우(자식 보고)에서는 손실이 관측되지 않았고, 다른 동작에서의 영향은 재지 않았다.
  - 분류기 envelope 계약을 통과한 요청은 replay 키를 만들지 않는다. 다른 보조 요청과 대화 요청의 replay 보호, 실패 응답의
    `X-Should-Retry: false`는 그대로다. 판정을 읽을 수 없을 때 native의 재질문은 backend에 간다(합성 backend 표본 1회에서 10번,
    그 호출은 실행되지 않음).
- **replay 차단 진단 기록**([#288](https://github.com/wotjr1649/Clauduct/issues/288), PR #290): 막힌 요청의 상태 기록에 먼저
  같은 키를 차지한 요청(`replayOf`: seq, 경과 ms, 규칙)을 남긴다. 본문과 지문은 남기지 않는다.
- **알려진 제한 원장 v0.6.10**(PR #292)
- 재측정 도구가 `$.ui`와 `$.ui.log` 문서 문구를 비교하고, 요약 TUI 회귀를 native 새 버전 절차와 발행 전 회귀에 넣었다
  (#286, 비공개 검증 도구).

## native

측정 기준은 v0.6.9와 같은 Claude Code 2.1.289다. 출하 시점 npm `latest`도 2.1.289였고, 무과금 재측정은 스냅샷 4개가 같았다.

## 출하 신원

| 항목 | 값 |
|---|---|
| 태그 | `v0.6.10` → `b47d7f8c415d00279b8bef93e68c678b999b245a` |
| `clauduct.exe` | `3b64b47123362fdf87f9b9e2f6b293981443b1290cf337dccd4544a4a73a19ad` |
| `install.ps1` | `ed4f70af63d46ae5972396c4d868166bc41202e165967f0fc1000693cf19ed68` |
| `uninstall.ps1` | `eb060a28e1f7d4ed30438774871798cb562bfb4dff4c9b53465b243a4ce06613` |
| `SHA256SUMS` | `fba81573b684329bbe0a2dbfa1b71443990a840ae1745cde53f214e0e5a865ab` |
| 빌드 | 깨끗한 태그 worktree, Go 1.27.1, `CGO_ENABLED=0`, `-trimpath`, 독립 캐시로 두 번 빌드해 바이트 일치 |

## 검증 (native 2.1.289 고정)

- **로컬**: gofmt·vet·build, 근거 태그 vet 2종, `go test -timeout=120m`과 `-race`(각 22개 패키지) 통과. 문서 인용 검사와 무과금
  재측정(fixture 포함) 통과. 작업마다 독립 리뷰를 받고 지적을 반영했다(#277 4건, #286 2건, #288 5건, #289 1건, 원장 1건, 출하 문서 4건).
- **실제 backend(이 릴리스 바이트)**: 기존 회귀 18개가 첫 실행에서 모두 통과했다. 검증 하네스는 실행마다 설치본 native를
  2.1.289와 sha256으로 대조했다.
- **실제 backend(신규 영향)**
  - 요약 요청 비용·지연·캐시 실측: luna·sol arm당 10회, astra·terra 2회. 요약을 비운 reasoning 기록의 재전송은 모델 4종 모두
    받았다.
  - TUI 요약 표본: 켠 세션 5회 모두 요약이 보였고, 끈 세션은 0줄이었다(첫 끈 표본 1회는 `/exit`가 native 보조 요청을 끊은 하네스
    시점 문제였다).
  - 분류기 표본: 수정 바이너리 5회, 릴리스 바이트 3회(위 18개에 포함) 통과.
  - 출하 전 첫 후보의 18개 회귀에서 `classifier-terra`가 1회 간헐 실패했다. 위 #288·#289로 원인을 찾아 고친 뒤 후보를 바꿨다.
- **합성 backend**: 요약 TUI 회귀(세 turn 모두 답보다 먼저 표시, native·backend 요청에 요약 text 0건, `--resume` 뒤 포함), 분류기
  오인 차단 재현(6/6)과 수정 후 차단 0(5/5), 분류기 실패 유형별 동작.
- **설치**: PowerShell 7.6.6에서 새 설치, v0.6.9 설치 후 교체·되돌리기, 설치본 세션(7회, 실패 0), 발행 후 검증(대상은 태그의
  commit), v0.6.9의 `--update --yes`, `.old` 정리, 두 번째 `--update` 무변경이 모두 통과했다.
- **이 머신의 실제 설치본**: v0.6.9에서 `--update --yes`로 v0.6.10이 됐다. digest 일치, `.old` 정리, 두 번째 `--update` 무변경,
  설정 변경 없음.

## 알려진 제한

- 남은 제한과 다시 볼 조건은 [COMPATIBILITY.md](COMPATIBILITY.md) 3절 "알려진 제한 원장 (v0.6.10)"에 있다. v0.6.10에서 새로
  생긴 항목은 다음과 같다.
  - 추론 요약은 main turn에서만 보인다(설계).
  - `--agent`로 띄운 main-thread agent의 요약은 요청 형태를 수집하지 않았다.
  - 요약 표시는 native의 `$.ui.log`에 기댄다(재측정과 회귀로 감지).
  - 분류기가 계속 읽을 수 없는 판정을 내면 native가 분류 요청을 더 보낸다(합성 backend 표본 1회에서 10번, 설계: native의 재질문).
- `gpt-5.5`의 실제 은퇴 뒤 동작은 2026-10-14T19:00Z 이후에 확인한다([#274](https://github.com/wotjr1649/Clauduct/issues/274)).
