# Security policy

## Reporting a vulnerability

Please report security problems privately by e-mail to **security@rintup.com**. Do not open a public
GitHub issue for them.

Include what you found, how to reproduce it and the provider version (`terraform version` lists it).
We confirm receipt and keep you updated while we work on a fix.

## Supported versions

Fixes are released as a new version of the latest release line. Upgrade to the newest
`rintuphq/rintup` release to receive them.

## Verifying releases

Every release ships `terraform-provider-rintup_<version>_SHA256SUMS` and a detached GPG signature
(`..._SHA256SUMS.sig`). `terraform init` verifies both against the signing key registered for the
`rintuphq` namespace in the Terraform Registry.
