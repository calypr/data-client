  if [ "$GOOS" == "linux" ]
  then
    set -e
    go build -o calypr-cli ./cmd/calypr-cli
    ls -al
    if [ "$GITHUB_PULL_REQUEST" == "false" ]; then
      zip calypr-cli_linux.zip calypr-cli && mv calypr-cli_linux.zip ~/shared/.
      aws s3 sync ~/shared s3://cdis-dc-builds/$GITHUB_BRANCH
    fi
    set +e
  elif [ "$GOOS" == "darwin" ]
  then
    set -e
    go build -o calypr-cli ./cmd/calypr-cli
    ls -al
    if [ "$GITHUB_PULL_REQUEST" == "false" ]; then
      zip calypr-cli_osx.zip calypr-cli && mv calypr-cli_osx.zip ~/shared/.
      aws s3 sync ~/shared s3://cdis-dc-builds/$GITHUB_BRANCH
    fi
    set +e
  elif [ "$GOOS" == "windows" ]
  then
    set -e
    go build -o calypr-cli.exe ./cmd/calypr-cli
    ls -al
    if [ "$GITHUB_PULL_REQUEST" == "false" ]; then
      zip calypr-cli_win64.zip calypr-cli.exe && mv calypr-cli_win64.zip ~/shared/.
      aws s3 sync ~/shared s3://cdis-dc-builds/$GITHUB_BRANCH
    fi
    set +e
  fi
