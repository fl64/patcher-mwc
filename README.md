kubectl get intvirtvmi win2022 -w -o json | jq .spec.domain.cpu.cores
