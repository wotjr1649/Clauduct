# v0.6.7 — 분류기 측정 결과 안내, 중단된 계수 분리

2026-10-03. [PR #246](https://github.com/wotjr1649/Clauduct/pull/246). 지원 범위와 측정 표는 [COMPATIBILITY.md](COMPATIBILITY.md)의
v0.6.7 절을 따른다.

## 바뀐 것

- **분류기 측정 결과 안내**
  - 계정이 제공하는 `classifier_model` pair가 Clauduct의 분류기 측정에서 통과하지 못했거나 측정하지 않은 pair면, 시작할 때
    stderr에 한 줄로 알리고 `session.classifierFinding`에 남긴다.
  - 사용은 막지 않는다(#238 계약). 공장값 `gpt-5.6-terra/low`는 출력이 없다.
  - 계정이 제공하지 않는 pair는 기존 설정 문제 줄로 알린다.
- **Luna/low 재측정**
  - 결과: 핵심 74개 중 위험 32개에서 2개(D08 Windows 작업 범위, D16 GitHub 승인)를 허용해 불합격. 독립 20개는 통과.
  - 측정 조건: 실제 backend, 2026-10-03, 114회.
  - 정정: 공개 문서의 "분류 품질은 Terra·Sol에서만 측정"은 틀렸다. Luna/low·medium은 2026-09-29에 이미 불합격이었다.
- **중단된 계수**
  - native가 버린 `count_tokens`(CANCELLED)를 gateway가 따로 센다(`requests.cancelledCounts`).
  - 종료 줄은 `refused`에서 이를 빼고 `counts_cancelled=N`으로 보여 주며, 이것만으로는 상태 JSON 전체를 출력하지 않는다.
  - 원래 집계와 생성 요청 취소(`cancelled_requests`) 보고는 그대로다.
- **문서**
  - 사용자 Claude Mods가 Clauduct 세션에서 로드·실행되는 것을 합성 backend로 확인해 적었다.
  - Anthropic 서버를 쓰는 native 기능의 지원 상태를 정리했다.

## 출하 신원

| 항목 | 값 |
|---|---|
| 태그 | `v0.6.7` → `62b84f2c59e56d5e216c57c1d58350fae624e923` |
| `clauduct.exe` | `58944ab3aa4f674661684c6c4ea933bca54ea1bbd4c09b14e7f8de9aa765afae` |
| `install.ps1` | `5a983cd794497d57c484e15542613cb6e861d133038523cd54942789e9051460` |
| `uninstall.ps1` | `ff339b7674a309e5add69d80b45aad7ad7980d00d5e1db04e348aa0a96e1d087` |
| `SHA256SUMS` | `26b1984a80d2c58baebdf70eca5edc05135d7062f129a87bbf920ba1727221da` |
| 빌드 | 깨끗한 태그 worktree, Go 1.27.1, `CGO_ENABLED=0`, `-trimpath`, 독립 캐시로 두 번 빌드해 바이트 일치 |

## 검증

- **로컬**
  - gofmt·vet·build, 근거 태그 vet 2종, `go test -timeout=120m`과 `-race`(각 22개 패키지) 통과. 문서 인용 검사 통과.
  - 독립 리뷰 지적 3건 중 2건을 반영했다(제공되지 않는 pair의 status 안내, 측정 표본 크기 서술). 재리뷰에서 결함은 없었다.
- **실제 backend(이 릴리스 바이트)**: 18개 시나리오 통과.
  - `/context` 직후 종료: 종료 줄 `refused=0 counts_cancelled=N`, 상태 출력 없음
  - Luna 분류기 세션: 안내 표시, 분류는 정상
  - Terra 분류기 세션: 안내 없음
  - v0.6.5 변경 4개
  - v0.6.4 회귀 12개
- **설치**: PowerShell 7.6.6에서 다음을 모두 통과했다.
  - 새 설치
  - v0.6.6 설치 후 교체·되돌리기
  - 설치본 세션(7회, 실패 0)
  - 발행 후 검증
  - `install.ps1 -Tag`
  - v0.6.6의 `--update --yes`
  - `.old` 정리
  - 두 번째 `--update` 무변경
- **이 머신의 실제 설치본**: v0.6.6에서 `--update --yes`로 v0.6.7이 됐다. digest 일치, 설정 변경 없음.

## 알려진 제한

- 분류기 측정은 일부 pair만 했다. Sol·Astra 등 대부분의 pair는 측정하지 않았고, 시작 안내로만 알린다.
- 사용자 mod를 원격으로 끄는 native 플래그가 telemetry를 끈 Clauduct 세션에서 어떻게 동작하는지는 확인하지 않았다. 내장 mod
  "You should know"는 native가 first-party·telemetry 켜짐 세션에만 제공하므로 쓸 수 없다.
- native changelog(2.1.280~2.1.288)에서 확인할 위험이 남아 있다.
  - 응답 도중 timeout 뒤 이어서 보내는 요청과 replay 차단
  - 첫 요청이 서버 출력 한도를 기다리는 시간
  - 분류기 transcript 압축
- v0.6.6 이전의 알려진 제한은 그대로다([v0.6.6](RELEASE-v0.6.6.md#알려진-제한)).
