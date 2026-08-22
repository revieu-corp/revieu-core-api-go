# RevieU Backend Scripts

This directory contains utility scripts for the RevieU backend project.

## 🔐 Secrets Management

### seal-secrets.sh

Encrypts secrets using Sealed Secrets before committing to Git.

**Usage:**
```bash
./scripts/seal-secrets.sh [service-name]
```

**Example:**
```bash
# Encrypt secrets for core service (default)
./scripts/seal-secrets.sh

# Encrypt secrets for another service
./scripts/seal-secrets.sh other-service
```

**What it does:**
1. Downloads the sealed secrets public key from the infra repo
2. Encrypts `apps/<service>/configs/secrets.yaml`
3. Outputs to `apps/<service>/configs/sealed-secrets.yaml`

**Requirements:**
- `kubeseal` CLI tool installed
- Internet connection to download public key

---

## 🪝 Git Hooks

### install-hooks.sh

Installs Git hooks to prevent committing unencrypted secrets.

**Usage:**
```bash
./scripts/install-hooks.sh
```

**What it does:**
- Installs a pre-commit hook that blocks commits containing `secrets.yaml` files
- Reminds developers to use `seal-secrets.sh` instead

**First-time setup:**
```bash
# Clone the repo
git clone git@github.com:revieu-corp/revieu-core-api-go.git
cd revieu-core-api-go

# Install hooks
./scripts/install-hooks.sh
```

---

## 📋 Workflow

### For new developers:

1. **Install hooks:**
   ```bash
   ./scripts/install-hooks.sh
   ```

2. **Create your secrets file:**
   ```bash
   export CLOUDFLARE_API_TOKEN="<CLOUDFLARE_API_TOKEN>"
   export JWT_SECRET="<JWT_SECRET>"
   ```

3. **Encrypt secrets:**
   ```bash
   ./scripts/seal-secrets.sh
   ```

4. **Commit the encrypted file:**
   ```bash
   git add apps/core/configs/sealed-secrets.yaml
   git commit -m "chore(core): update sealed secrets"
   git push
   ```

### For updating secrets:

1. **Edit secrets.yaml:**
   ```bash
   vim apps/core/configs/secrets.yaml
   ```

2. **Re-encrypt:**
   ```bash
   ./scripts/seal-secrets.sh
   ```

3. **Commit:**
   ```bash
   git add apps/core/configs/sealed-secrets.yaml
   git commit -m "chore(core): update sealed secrets"
   git push
   ```

---

## 🔒 Security Notes

- ✅ `sealed-secrets.yaml` - Safe to commit (encrypted)
- ❌ `secrets.yaml` - Never commit (unencrypted, in .gitignore)
- ✅ `pub-cert.pem` - Safe to commit (public key)

The pre-commit hook will prevent you from accidentally committing unencrypted secrets.
