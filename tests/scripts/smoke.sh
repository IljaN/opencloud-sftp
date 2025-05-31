#!/bin/bash

RAND_FILE="$(mktemp --suffix=.bin -p ./ )"
head -c 10M /dev/urandom > "$RAND_FILE"
SHA1_HASH_ORIG=$(sha1sum "$RAND_FILE" | awk '{print $1}')
echo "$RAND_FILE"

sshpass -p '1' sftp -P 2222 admin@127.0.0.1 <<EOF
pwd
ls -la
ls -la Admin
cd Admin
pwd
ls -la
put $RAND_FILE
get $RAND_FILE download.bin
ls -la
ls FolderBaz
bye
EOF

SHA1_HASH_DL=$(sha1sum "./download.bin" | awk '{print $1}')

echo "$SHA1_HASH_ORIG"
echo "$SHA1_HASH_DL"

if [[ "$SHA1_HASH_ORIG" == "$SHA1_HASH_DL" ]]; then
  echo "Hashes after upload/download match"
fi


#rm download.bin
