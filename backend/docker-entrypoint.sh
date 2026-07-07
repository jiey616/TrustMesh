#!/bin/sh
set -eu

knowledge_dir="${KNOWLEDGE_STORAGE_PATH:-/var/lib/trustmesh-knowledge}"
files_dir="${FILES_STORAGE_PATH:-/var/lib/trustmesh-files}"
trustmesh_uid="$(id -u trustmesh)"
trustmesh_gid="$(id -g trustmesh)"

mkdir -p "${knowledge_dir}"
mkdir -p "${files_dir}"

# Ensure knowledge directory ownership.
current_owner="$(stat -c '%u:%g' "${knowledge_dir}")"
expected_owner="${trustmesh_uid}:${trustmesh_gid}"

if [ "${current_owner}" != "${expected_owner}" ]; then
  chown -R trustmesh:trustmesh "${knowledge_dir}"
fi

# Ensure files directory ownership.
current_files_owner="$(stat -c '%u:%g' "${files_dir}")"
if [ "${current_files_owner}" != "${expected_owner}" ]; then
  chown -R trustmesh:trustmesh "${files_dir}"
fi

exec su-exec trustmesh "$@"
