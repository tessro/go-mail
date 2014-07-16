#!/bin/bash

set -e

wget ftp://ftp.internic.net/domain/root.zone.gz
gunzip root.zone.gz

OUT=tlds.go

printf "package mail\n\nvar tlds = []string{" > $OUT
sed -e 's/;.*//' < root.zone -e 's/\. / /' | \
        expand -8 | \
        tr '[A-Z]' '[a-z]' | \
        awk '/. ns / {print "\"" $1 "\",\n" }' | \
        sort -u | \
        sort -nrs -k2 >> $OUT
echo "}" >> $OUT
gofmt -w $OUT
