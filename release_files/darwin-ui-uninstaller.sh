#!/bin/sh

export PATH=$PATH:/usr/local/bin

# check if anonbird is installed
NB_BIN=$(which anonbird)
if [ -z "$NB_BIN" ]
then
  exit 0
fi
# start anonbird daemon service
echo "anonbird daemon service still running. You can uninstall it by running: "
echo "sudo anonbird service stop"
echo "sudo anonbird service uninstall"
