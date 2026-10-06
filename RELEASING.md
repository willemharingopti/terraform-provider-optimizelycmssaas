# Releasing to the Terraform Registry

The registry reads releases from GitHub. A release is a **signed**, semantically versioned tag whose assets are built by
GoReleaser (`.goreleaser.yml`) from the workflow in `.github/workflows/release.yml`.

## One-time setup

1. **Repository.** It must be a public GitHub repository named `terraform-provider-optimizelycmssaas`. The registry
   address is `willemharingopti/optimizelycmssaas`, which is the GitHub user (or organisation) that owns the repository.
2. **Module path.** `go.mod` and the imports must use the real path, `github.com/willemharingopti/terraform-provider-optimizelycmssaas`.
   The registry address also appears in `main.go`, the examples, the docs and the guides.
3. **GPG signing key.** Releases are signed so Terraform can verify them.

   ```sh
   gpg --full-generate-key                      # choose RSA, 4096 bits; use a strong passphrase
   gpg --list-secret-keys --keyid-format=long   # note the key ID
   gpg --armor --export <KEY ID> > public.asc   # the PUBLIC key, for the registry
   gpg --armor --export-secret-keys <KEY ID>    # the PRIVATE key, for the GitHub secret below
   ```

   Keep the private key and passphrase out of the repository, shell history and chat. Store a backup in your password
   manager, and delete any exported private-key file once it is saved as a secret.
4. **GitHub secrets and environment.** In the repository settings, create an environment named `release`, add a required
   reviewer to it, and add two secrets *to that environment*: `GPG_PRIVATE_KEY` (the armored private key) and `PASSPHRASE`.
   `GITHUB_TOKEN` is supplied automatically. Also protect `v*` tags with a ruleset, so only maintainers can create them.
5. **Registry.** Sign in at <https://registry.terraform.io> with GitHub, open *User Settings > Signing Keys* and add the
   **public** key for your namespace. Then choose *Publish > Provider*, and select the repository. The registry installs a
   webhook that notices new releases.

## Making a release

1. Update `CHANGELOG.md` and make sure the checks pass: `make test`, and `make docs` leaves no changes in `docs/`.
2. Optionally rehearse the build, which signs nothing and publishes nothing: `make snapshot`.
3. Tag and push. The version must be `v` followed by a semantic version:

   ```sh
   git tag v0.1.0
   git push origin v0.1.0
   ```

4. Approve the `release` environment when the workflow asks. It builds the zips, writes `..._SHA256SUMS` and its
   `.sig`, attaches `..._manifest.json`, and creates the GitHub release.
5. The registry picks the release up within minutes. A version, once ingested, cannot be replaced: if something is wrong,
   publish the next version (for example `v0.1.1`) instead of re-tagging.

## Checking a release

After the registry shows the version, in an empty directory:

```sh
cat > main.tf <<'EOF'
terraform {
  required_providers {
    optimizelycmssaas = { source = "willemharingopti/optimizelycmssaas", version = "0.1.0" }
  }
}
EOF
terraform init
```

`terraform init` downloads the provider, checks its signature, and writes `.terraform.lock.hcl`.

## Documentation

`docs/` is what the registry displays. The reference pages are generated (`make docs`); edit the schema descriptions,
`examples/` or `templates/guides/` and regenerate, never the generated files. CI fails when `docs/` is out of date.
