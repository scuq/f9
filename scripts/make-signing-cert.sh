#!/usr/bin/env bash
# Create the self-signed Windows code-signing certificate used by the release
# workflow. Run once, locally; never commit the outputs.
#
#   bash scripts/make-signing-cert.sh [outdir]   (default: ./signing, gitignored)
#
# Produces:
#   f9-codesign.pfx  private key + cert  -> GitHub secret WIN_SIGN_PFX_B64 (base64)
#   f9-codesign.cer  public cert (DER)   -> shipped with every Windows release so
#                                           users can import it manually
#   pfx password                         -> GitHub secret WIN_SIGN_PFX_PASSWORD
#
# Set the two secrets:
#   gh secret set WIN_SIGN_PFX_B64      < <(base64 -w0 signing/f9-codesign.pfx)
#   gh secret set WIN_SIGN_PFX_PASSWORD < signing/password.txt
set -euo pipefail

out="${1:-signing}"
days="${F9_CERT_DAYS:-3650}"
subject="${F9_CERT_SUBJECT:-/CN=f9 (self-signed)/O=f9}"
mkdir -p "$out"
chmod 700 "$out"

if [ -e "$out/f9-codesign.pfx" ]; then
  echo "refusing to overwrite $out/f9-codesign.pfx" >&2
  exit 1
fi

pass="$(openssl rand -base64 24)"
printf '%s' "$pass" > "$out/password.txt"
chmod 600 "$out/password.txt"

# Code-signing certificate: EKU codeSigning, digitalSignature only.
openssl req -x509 -newkey rsa:4096 -sha256 -days "$days" -nodes \
  -subj "$subject" \
  -addext "keyUsage=critical,digitalSignature" \
  -addext "extendedKeyUsage=critical,codeSigning" \
  -addext "basicConstraints=critical,CA:FALSE" \
  -keyout "$out/f9-codesign.key" -out "$out/f9-codesign.crt" >/dev/null 2>&1

# PFX for signtool (legacy ciphers keep it readable by Windows' PFX importer).
openssl pkcs12 -export -legacy \
  -inkey "$out/f9-codesign.key" -in "$out/f9-codesign.crt" \
  -name "f9 code signing" -passout "pass:$pass" -out "$out/f9-codesign.pfx" 2>/dev/null \
|| openssl pkcs12 -export \
  -inkey "$out/f9-codesign.key" -in "$out/f9-codesign.crt" \
  -name "f9 code signing" -passout "pass:$pass" -out "$out/f9-codesign.pfx"
openssl x509 -in "$out/f9-codesign.crt" -outform DER -out "$out/f9-codesign.cer"
chmod 600 "$out"/f9-codesign.*

echo "wrote:"
ls -1 "$out"
echo
echo "fingerprint (SHA-256):"
openssl x509 -in "$out/f9-codesign.crt" -noout -fingerprint -sha256
echo
echo "next:"
echo "  gh secret set WIN_SIGN_PFX_B64      < <(base64 -w0 $out/f9-codesign.pfx)"
echo "  gh secret set WIN_SIGN_PFX_PASSWORD < $out/password.txt"
