#!/bin/sh

export PATH=$PATH:/usr/local/bin:/opt/homebrew/bin

# check if anonbird is installed
NB_BIN=$(which anonbird)
if [ -n "$NB_BIN" ]
then
  echo "Stopping and uninstalling AnonBird daemon"
  anonbird service stop || true
  anonbird service uninstall || true
fi

# check if anonbird is installed
NB_BIN=$(which anonbird)
if [ -z "$NB_BIN" ]
then
  echo "AnonBird daemon is not installed. Install anonbird first or set ANONBIRD_HOMEBREW_FORMULA for Homebrew installs."
  exit 1
fi
NB_UI_VERSION=$1
NB_VERSION=$(anonbird version)
if [ "X-$NB_UI_VERSION" != "X-$NB_VERSION" ]
then
  echo "AnonBird daemon is running with a different version than the AnonBird UI:"
  echo "AnonBird UI Version: $NB_UI_VERSION"
  echo "AnonBird Daemon Version: $NB_VERSION"
  echo "Please update anonbird from the same AnonBird release channel."
fi

if [ -n "$NB_BIN" ]
then
  echo "Stopping AnonBird daemon"
  osascript -e 'quit app "AnonBird UI"' 2> /dev/null || true
  anonbird service stop 2> /dev/null || true
fi

# start anonbird daemon service
echo "Starting AnonBird daemon"
anonbird service install 2> /dev/null || true
anonbird service start || true

# start app
open /Applications/AnonBird\ UI.app
