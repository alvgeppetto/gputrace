# Makefile for gputrace development
# This file is excluded from git via .git/info/exclude

UPSTREAM_REPO := ~/go/src/github.com/tmc/mlx-go
UPSTREAM_PREFIX := experiments/gputrace
UPSTREAM_BRANCH := gputrace-split

GPUTRACE_APP := $(HOME)/go/bin/gputrace.app
AXPERMS_APP := $(HOME)/go/bin/axperms.app
AXPERMS_BIN := $(HOME)/go/bin/axpermsbak
BUNDLE_ID := com.tmc.gputrace
AXPERMS_BUNDLE_ID := com.github.tmc.gputrace.axperms

.PHONY: all build install clean rebuild test-permissions reset-permissions axperms setup-axperms help pull-upstream refresh-split

all: build

build:
	go install ./cmd/gputrace


# Fetch test data for full test suite
fetch-testdata:
	git checkout test-assets -- testdata

install: clean build setup-permissions
	@echo "Reinstall complete with fresh permissions"

reinstall: clean build setup-permissions

# Clean app bundle to force macgo to recreate it
clean:
	rm -rf $(GPUTRACE_APP)

# Setup permissions after clean rebuild
setup-permissions:
	@echo "Step 1: Triggering bundle creation..."
	-gputrace xp check-status --no-prompt 2>/dev/null || true
	@echo "Step 2: Resetting TCC for Accessibility (clears stale code requirement)..."
	-tccutil reset Accessibility $(BUNDLE_ID) 2>/dev/null || true
	@echo "Step 3: Resetting TCC for Screen Recording..."
	-tccutil reset ScreenCapture $(BUNDLE_ID) 2>/dev/null || true
	@echo "Step 4: Re-triggering permission prompt (adds app to list with fresh signature)..."
	-gputrace xp check-status --no-prompt 2>/dev/null || true
	# -gputrace xp screenshot --no-prompt 2>/dev/null || true
	@sleep 2
	@echo "Step 5: Opening System Settings Accessibility pane..."
	$(AXPERMS_BIN) -open 2>/dev/null || true
	@sleep 2
	@echo "Step 6: Enabling accessibility permission..."
	$(AXPERMS_BIN) -enable gputrace.app 2>/dev/null | grep -v "macgo:" || true
	@sleep 2
	# @echo "Step 7: Enabling screen recording permission..."
	# $(AXPERMS_BIN) -enable-screen-recording gputrace.app 2>/dev/null | grep -v "macgo:" || true
	@echo "Step 8: Verifying permissions..."
	@gputrace xp check-status --no-prompt && echo "✓ Accessibility OK" || echo "✗ Accessibility permission may need manual intervention"
	# @gputrace xp screenshot --no-prompt -o /tmp/test-screenshot.png 2>/dev/null && echo "✓ Screen Recording OK" || echo "✗ Screen Recording permission may need manual setup in System Settings > Privacy & Security > Screen Recording"

# Full permission reset (use when TCC database is stale)
reset-permissions:
	@echo "Resetting TCC entries..."
	tccutil reset Accessibility $(BUNDLE_ID) 2>/dev/null || true
	tccutil reset ScreenCapture $(BUNDLE_ID) 2>/dev/null || true
	tccutil reset Accessibility $(AXPERMS_BUNDLE_ID) 2>/dev/null || true
	@echo "Re-triggering permission prompts..."
	-$(AXPERMS_BIN) -prompt 2>&1 | grep -v "macgo:" || true
	-gputrace xp check-status --no-prompt 2>/dev/null || true
	-gputrace xp screenshot --no-prompt -o /tmp/test-screenshot.png 2>/dev/null || true
	@echo ""
	@echo "Please manually enable axperms and gputrace in System Settings,"
	@echo "then run 'make setup-permissions'"

reset: clean build setup-permissions

# Quick test that permissions work
test-permissions:
	gputrace xp check-status --no-prompt

# Build axperms helper and update bundle
axperms:
	go build -o $(AXPERMS_BIN) ./cmd/axperms
	@# Update the binary inside the app bundle if it exists
	@if [ -d "$(AXPERMS_APP)/Contents/MacOS" ]; then \
		cp $(AXPERMS_BIN) $(AXPERMS_APP)/Contents/MacOS/axperms; \
	fi

# First-time setup for axperms - requires manual user action
# Run this ONCE before using axperms to manage permissions
setup-axperms: axperms
	@echo "Setting up axperms Accessibility permission..."
	@echo "This is a ONE-TIME setup - axperms needs Accessibility permission"
	@echo "to manipulate System Settings UI for other apps."
	@echo ""
	@echo "Resetting any stale axperms TCC entry..."
	-tccutil reset Accessibility $(AXPERMS_BUNDLE_ID) 2>/dev/null || true
	@echo ""
	@echo "Triggering permission prompt..."
	@# Run axperms to trigger the prompt - it will fail but add itself to the list
	-$(AXPERMS_BIN) -prompt 2>&1 | grep -v "macgo:" || true
	@echo ""
	@echo "============================================"
	@echo "ACTION REQUIRED:"
	@echo "1. System Settings should now be open to Privacy & Security > Accessibility"
	@echo "2. Find 'axperms' in the list"
	@echo "3. Toggle it ON"
	@echo "4. You may need to authenticate with your password"
	@echo "5. Then run 'make setup-permissions' to configure gputrace"
	@echo "============================================"

help:
	@echo "gputrace Makefile"
	@echo ""
	@echo "Development targets:"
	@echo "  build              - Build gputrace"
	@echo "  rebuild            - Clean app bundle and rebuild with fresh permissions"
	@echo "  clean              - Remove app bundle (forces macgo to recreate)"
	@echo ""
	@echo "Permission setup (run in order for first-time setup):"
	@echo "  setup-axperms      - ONE-TIME: Grant axperms Accessibility (manual step)"
	@echo "  setup-permissions  - Setup gputrace Accessibility + Screen Recording"
	@echo "  reset-permissions  - Full TCC reset + setup (for stale permissions)"
	@echo "  test-permissions   - Quick test that permissions work"
	@echo ""
	@echo "Helper tools:"
	@echo "  axperms            - Build axperms helper tool"
	@echo ""
	@echo "Upstream sync targets:"
	@echo "  pull-upstream      - Pull new commits from upstream subtree"
	@echo "  refresh-split      - Re-run subtree split (creates new commits)"

# Pull new commits from the upstream subtree
pull-upstream:
	@echo "Pulling from $(UPSTREAM_REPO) branch $(UPSTREAM_BRANCH)..."
	git pull $(UPSTREAM_REPO) $(UPSTREAM_BRANCH)

# Re-run the subtree split to include new commits from mlx-go
refresh-split:
	@echo "Running subtree split in $(UPSTREAM_REPO)..."
	cd $(UPSTREAM_REPO) && git subtree split --prefix=$(UPSTREAM_PREFIX) -b $(UPSTREAM_BRANCH)
	@echo "Now run 'make pull-upstream' to fetch the new commits"
