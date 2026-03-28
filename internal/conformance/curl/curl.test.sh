#### basic GET
curl -s "${GBASH_CONFORMANCE_CURL_BASE_URL}/plain"

#### redirect follow
curl -s -L "${GBASH_CONFORMANCE_CURL_BASE_URL}/redirect"

#### auth and request headers
curl -s -u user:pass -A 'agent/1.0' -e https://ref.example -b 'a=1; b=2' "${GBASH_CONFORMANCE_CURL_BASE_URL}/inspect/request"

#### data body
curl -s -d 'hello world' "${GBASH_CONFORMANCE_CURL_BASE_URL}/echo/body"

#### data urlencode
curl -s --data-urlencode 'message=hello world' "${GBASH_CONFORMANCE_CURL_BASE_URL}/echo/body"

#### multipart form
upload_file=/tmp/gbash-curl-multipart-upload.txt
printf '%s' 'upload payload' >"$upload_file"
curl -s -F "file=@${upload_file};type=text/plain" "${GBASH_CONFORMANCE_CURL_BASE_URL}/inspect/form"

#### upload file
upload_file=/tmp/gbash-curl-put-upload.txt
printf '%s' 'upload payload' >"$upload_file"
curl -s -T "$upload_file" "${GBASH_CONFORMANCE_CURL_BASE_URL}/echo/body"

#### output file
output_file=/tmp/gbash-curl-output-file.txt
curl -s -o "$output_file" "${GBASH_CONFORMANCE_CURL_BASE_URL}/files/report.txt"
cat "$output_file"

#### remote name
curl -s -O "${GBASH_CONFORMANCE_CURL_BASE_URL}/files/report.txt"
cat report.txt

#### write out with output file
output_file=/tmp/gbash-curl-write-out-file.txt
curl -s -o "$output_file" -w '%{http_code} %{content_type} %{url_effective} %{size_download}' "${GBASH_CONFORMANCE_CURL_BASE_URL}/files/report.txt"
printf '\n'
cat "$output_file"

#### include headers
response="$(curl -s -i "${GBASH_CONFORMANCE_CURL_BASE_URL}/include")"
normalized="$(printf '%s\n' "$response" | sed 's/\r$//' | grep -Ev '^(Date: |Content-Length: )')"
expected='HTTP/1.1 200 OK
Content-Type: text/plain
X-Test: include

included-body'
[ "$normalized" = "$expected" ]
printf '%s\n' "$normalized"

#### head request
response="$(curl -s -I "${GBASH_CONFORMANCE_CURL_BASE_URL}/head")"
normalized="$(printf '%s\n' "$response" | sed 's/\r$//' | grep -Ev '^(Date: |Content-Length: )')"
expected='HTTP/1.1 200 OK
Content-Type: text/plain
X-Test: head-only'
[ "$normalized" = "$expected" ]
printf '%s\n' "$normalized"

#### fail with silent show-error
set +e
stderr="$(curl -f -sS "${GBASH_CONFORMANCE_CURL_BASE_URL}/status/404" 2>&1)"
status=$?
set -e
printf '%s\n' "$status"
printf '%s\n' "$stderr"
