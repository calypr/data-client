#!/usr/bin/env bash
#
# Adapted from 'How To Build Go Executables for Multiple Platforms on Ubuntu 16.04'
# By Marko Mudrinić
#
# Usage: 
#   ./build.sh

if [ "$1" == "-h" ] || [ "$1" == "--help" ]; then
  echo "usage: $0"
  echo "output: zipped executables to ./build directory"
fi

package=$1

if [[ -z "$package" ]]; then
  package='gen3-client'
fi

package_split=(${package//\// })
package_name=${package_split[-1]}
  
platforms=(
  "darwin/arm64"
  "darwin/amd64"
  "linux/amd64"
  "windows/amd64"
)

mkdir -p ./build
> checksums.txt
for platform in "${platforms[@]}"
do
  platform_split=(${platform//\// })
  GOOS=${platform_split[0]}
  GOARCH=${platform_split[1]}
  output_name=$package_name'-'$GOOS'-'$GOARCH
  exe_name=$package_name

  if [ $GOOS = "windows" ]; then
    exe_name+='.exe'

  elif [ $GOOS = "darwin" ]; then
    if [ $GOARCH = "arm64" ]; then
      output_name=$package_name'-macos'

    elif [ $GOARCH = "amd64" ]; then
      output_name=$package_name'-macos-intel'
    fi
  fi	

  printf 'Building %s...' "$output_name"
  env GOOS=$GOOS GOARCH=$GOARCH go build -o ./build/$exe_name .
  cd build
  zip -r -q $output_name $exe_name 
  sha256sum $output_name.zip >> checksums.txt
  cd ..

  if [ $? -ne 0 ]; then
       echo 'An error has occurred! Aborting the script execution...'
    exit 1
  fi
  echo 'OK'
done

# Clean up build artifacts
rm build/{$package_name,$package_name.exe}

