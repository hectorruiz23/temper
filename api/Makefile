GO ?= go

.PHONY: build verify accept accept-container test install-hooks check-github

build:
	@mkdir -p build
	CGO_ENABLED=0 $(GO) build -trimpath -o build/temper ./cmd/temper

verify:
	@files="$$(gofmt -l .)"; if [ -n "$$files" ]; then echo "gofmt found unformatted files:"; echo "$$files"; exit 1; fi
	$(GO) vet ./...
	$(GO) test ./...

accept:
	$(GO) test -count=1 ./acceptance/...

accept-container:
	$(GO) test -count=1 -tags container ./acceptance/...

test:
	$(GO) test ./...

install-hooks:
	@common="$$(git rev-parse --git-common-dir)"; \
	case "$$common" in \
		/*) ;; \
		*) common="$$(cd "$(CURDIR)/$$common" && pwd)";; \
	esac; \
	mkdir -p "$$common/hooks"; \
	ln -sf "$(CURDIR)/scripts/pre-commit" "$$common/hooks/pre-commit"; \
	chmod +x "$(CURDIR)/scripts/pre-commit"; \
	echo "installed pre-commit hook in $$common/hooks/pre-commit"

check-github:
	./scripts/check-github
