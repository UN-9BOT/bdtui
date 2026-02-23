# Post-Release Verification Checklist

Use this checklist right after every published release.

## 1) Artifacts presence

- [ ] Release exists for expected date tag.
- [ ] All binaries are present:
  - [ ] `bdtui-darwin-amd64`
  - [ ] `bdtui-darwin-arm64`
  - [ ] `bdtui-linux-amd64`
  - [ ] `bdtui-linux-arm64`
- [ ] `checksums.txt` is attached.

## 2) Checksums integrity

- [ ] Download binaries and `checksums.txt`.
- [ ] Verify checksums:

```bash
sha256sum -c checksums.txt --ignore-missing
```

- [ ] No checksum mismatches.

## 3) Binary smoke tests

- [ ] Linux binary starts: `./bdtui-linux-amd64 --help` (or arm64 build on arm64 host).
- [ ] macOS binary starts: `./bdtui-darwin-arm64 --help` (or amd64 build on amd64 host).
- [ ] App launches without immediate runtime crash.

## 4) Source build verification

- [ ] README source build commands are current and executable:

```bash
go test ./...
go build ./...
./bdtui --help
```

- [ ] No undocumented prerequisite was required.

## 5) Record outcome

- [ ] Add verification note to release notes/runbook log.
- [ ] If issues found, create follow-up beads and reference release tag.
