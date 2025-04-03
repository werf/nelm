# Nelm usage

- [Encrypted values files](#encrypted-values-files)
- [Encrypted arbitrary files](#encrypted-arbitrary-files)

## Encrypted values files

Values files can be encrypted and stored in a Helm chart or a git repo. Such values files are decrypted in-memory during templating.

Create a secret key:
```bash
export NELM_SECRET_KEY="$(nelm chart secret key create)"
```

Create a new secret-values file:
```bash
nelm chart secret values-file edit secret-values.yaml
```
... with the following content:
```yaml
password: verysecurepassword123
```

Reference encrypted value in Helm templates:
```yaml
password: {{ .Values.password }}
```

Render the chart:
```bash
nelm chart render
```
```yaml
password: verysecurepassword123
```

NOTE: `$NELM_SECRET_KEY` must be set for any command that encrypts/decrypts secrets, including `nelm chart render`.

## Encrypted arbitrary files

Arbitrary files can be encrypted and stored in the `secret/` directory of a Helm chart. Such files are decrypted in-memory during templating.

Create a secret key:
```bash
export NELM_SECRET_KEY="$(nelm chart secret key create)"
```

Create a new secret file:
```bash
nelm chart secret file edit secret/config.yaml
```
... with the following content:
```yaml
user: john-doe
password: verysecurepassword123
```

Reference encrypted secret in Helm templates:
```yaml
config: {{ werf_secret_file "config.yaml" | nindent 4 }}
```

Render the chart:
```bash
nelm chart render
```
```yaml
config:
  user: john-doe
  password: verysecurepassword123
```
