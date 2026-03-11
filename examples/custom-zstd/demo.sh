printf 'custom commands are runtime extensions\n' > message.txt
zstd -o message.zst message.txt
zstd -d -o roundtrip.txt message.zst
cat roundtrip.txt
zstd --about
zstd-lazy --about
