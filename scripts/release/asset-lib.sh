#!/usr/bin/env bash

release_archive_ext() {
  local goos="$1"
  if [[ "$goos" == "windows" ]]; then
    echo "zip"
    return
  fi
  echo "tar.gz"
}

release_extract_dir_name() {
  local version="$1"
  local goos="$2"
  local goarch="$3"
  echo "igw_${version}_${goos}_${goarch}"
}

release_versioned_asset_name() {
  local version="$1"
  local goos="$2"
  local goarch="$3"
  local ext
  ext="$(release_archive_ext "$goos")"
  echo "$(release_extract_dir_name "$version" "$goos" "$goarch").${ext}"
}

release_latest_alias_name() {
  local goos="$1"
  local goarch="$2"
  local ext
  ext="$(release_archive_ext "$goos")"
  echo "igw_${goos}_${goarch}.${ext}"
}
