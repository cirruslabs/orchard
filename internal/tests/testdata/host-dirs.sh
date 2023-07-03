#!/bin/sh

if test -d "/Volumes/My Shared Files/readwrite"
then
  echo "Read-write mount exists"
else
  echo "Read-write mount not found"

  exit 1
fi

if test -d "/Volumes/My Shared Files/readonly"
then
  echo "Read-only mount exists"
else
  echo "Read-only mount not found"

  exit 1
fi

touch "/Volumes/My Shared Files/readonly/test-ro.txt" 2>/dev/null || echo "Failed to create a file in read-only mount"
touch "/Volumes/My Shared Files/readwrite/test-rw.txt" && echo "Successfully created a file in read-write mount"
