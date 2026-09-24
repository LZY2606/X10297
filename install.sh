#!/usr/bin/env sh
# Modified version of: https://github.com/starship/starship/blob/master/install/install.sh

set -eu
printf '\n'

BOLD="$(tput bold 2>/dev/null || printf '')"
GREY="$(tput setaf 0 2>/dev/null || printf '')"
UNDERLINE="$(tput smul 2>/dev/null || printf '')"
RED="$(tput setaf 1 2>/dev/null || printf '')"
GREEN="$(tput setaf 2 2>/dev/null || printf '')"
YELLOW="$(tput setaf 3 2>/dev/null || printf '')"
BLUE="$(tput setaf 4 2>/dev/null || printf '')"
MAGENTA="$(tput setaf 5 2>/dev/null || printf '')"
NO_COLOR="$(tput sgr0 2>/dev/null || printf '')"

SUPPORTED_TARGETS="linux_386 linux_amd64 linux_arm64 linux_armv6 \
                   darwin_amd64 darwin_arm64 \
                   windows_amd64"

info() {
  printf '%s\n' "${BOLD}${GREY}>${NO_COLOR} $*"
}

warn() {
  printf '%s\n' "${YELLOW}! $*${NO_COLOR}"
}

error() {
  printf '%s\n' "${RED}x $*${NO_COLOR}" >&2
}

completed() {
  printf '%s\n' "${GREEN}✓${NO_COLOR} $*"
}

has() {
  command -v "$1" 1>/dev/null 2>&1
}

curl_is_snap() {
  curl_path="$(command -v curl)"
  case "$curl_path" in
  /snap/*) return 0 ;;
  *) return 1 ;;
  esac
}

# Make sure user is not using zsh or non-POSIX-mode bash, which can cause issues
verify_shell_is_posix_or_exit() {
  if [ -n "${ZSH_VERSION+x}" ]; then
    error "Running installation script with \`zsh\` is known to cause errors."
    error "Please use \`sh\` instead."
    exit 1
  elif [ -n "${BASH_VERSION+x}" ] && [ -z "${POSIXLY_CORRECT+x}" ]; then
    error "Running installation script with non-POSIX \`bash\` may cause errors."
    error "Please use \`sh\` instead."
    exit 1
  else
    true # No-op: no issues detected
  fi
}

get_tmpfile() {
  suffix="$1"
  if has mktemp; then
    printf "%s.%s" "$(mktemp)" "${suffix}"
  else
    # No really good options here--let's pick a default + hope
    printf "/tmp/tasker.%s" "${suffix}"
  fi
}

# Test if a location is writable by trying to write to it. Windows does not let
# you test writability other than by writing: https://stackoverflow.com/q/1999988
test_writeable() {
  path="${1:-}/.tasker_write_test_$$"
  if touch "${path}" 2>/dev/null; then
    rm "${path}"
    return 0
  else
    return 1
  fi
}

download() {
  file="$1"
  url="$2"

  if has curl && curl_is_snap; then
    warn "curl installed through snap cannot download tasker."
    warn "See https://github.com/adhocore/gronx/issues for details."
    warn "Searching for other HTTP download programs..."
  fi

  if has curl && ! curl_is_snap; then
    cmd="curl --fail --silent --location --output $file $url"
  elif has wget; then
    cmd="wget --quiet --output-document=$file $url"
  elif has fetch; then
    cmd="fetch --quiet --output=$file $url"
  else
    error "No HTTP download program (curl, wget, fetch) found, exiting…"
    return 1
  fi

  $cmd && return 0 || rc=$?

  error "Command failed (exit code $rc): ${BLUE}${cmd}${NO_COLOR}"
  printf "\n" >&2
  info "This is likely due to tasker not yet supporting your configuration."
  info "If you would like to see a build for your configuration,"
  info "please create an issue requesting a build for ${MAGENTA}${TARGET}${NO_COLOR}:"
  info "${BOLD}${UNDERLINE}https://github.com/adhocore/gronx/issues/new/${NO_COLOR}"
  return $rc
}

install() {
  bin_dir=$1
  sudo=${2-}

  if test_writeable "${bin_dir}"; then
    sudo=""
    msg="Installing tasker, please wait…"
  else
    warn "Escalated permissions are required to install to ${bin_dir}"
    elevate_priv
    sudo="sudo"
    msg="Installing tasker as root, please wait…"
  fi
  info "$msg"

  tmp_dir=$(mktemp -d)

  # download to the temp directory
  download "${tmp_dir}/${ARCHIVE_FILE}" "${URL}"

  # extract the archive
  case "${ARCHIVE_FILE}" in
    *.tar.gz)
      tar -xzf "${tmp_dir}/${ARCHIVE_FILE}" -C "${tmp_dir}" --strip-components=1
      ;;
    *.zip)
      if has unzip; then
        unzip -q "${tmp_dir}/${ARCHIVE_FILE}" -d "${tmp_dir}"
        # Find the extracted directory and move contents up
        extracted_dir=$(find "${tmp_dir}" -mindepth 1 -maxdepth 1 -type d | head -1)
        if [ -n "${extracted_dir}" ]; then
          mv "${extracted_dir}"/* "${tmp_dir}/"
          rm -rf "${extracted_dir}"
        fi
      else
        error "unzip is required to extract the archive but was not found"
        exit 1
      fi
      ;;
    *)
      error "Unsupported archive format: ${ARCHIVE_FILE}"
      exit 1
      ;;
  esac

  # make the binary executable
  chmod +x "${tmp_dir}/tasker"

  # move the binary to the bin dir, using sudo if required
  ${sudo} mv "${tmp_dir}/tasker" "${bin_dir}/tasker"

  # cleanup
  rm -rf "${tmp_dir}"
}

usage() {
  printf "%s\n" \
    "install.sh [option]" \
    "" \
    "Fetch and install the latest version of tasker, if tasker is already" \
    "installed it will be updated to the latest version."

  printf "\n%s\n" "Options"
  printf "\t%s\n\t\t%s\n\n" \
    "-V, --verbose" "Enable verbose output for the installer" \
    "-f, -y, --force, --yes" "Skip the confirmation prompt during installation" \
    "-p, --platform" "Override the platform identified by the installer [default: ${PLATFORM}]" \
    "-b, --bin-dir" "Override the bin installation directory [default: ${BIN_DIR}]" \
    "-a, --arch" "Override the architecture identified by the installer [default: ${ARCH}]" \
    "-B, --base-url" "Override the base URL used for downloading releases [default: ${BASE_URL}]" \
    "-v, --version" "Install a specific version of tasker [default: ${VERSION}]" \
    "-h, --help" "Display this help message"
}

elevate_priv() {
  if ! has sudo; then
    error 'Could not find the command "sudo", needed to get permissions for install.'
    info "If you are on Windows, please run your shell as an administrator, then"
    info "rerun this script. Otherwise, please run this script as root, or install"
    info "sudo."
    exit 1
  fi
  if ! sudo -v; then
    error "Superuser not granted, aborting installation"
    exit 1
  fi
}

# Currently supporting:
#   - win (Git Bash)
#   - darwin
#   - linux
detect_platform() {
  platform="$(uname -s | tr '[:upper:]' '[:lower:]')"

  case "${platform}" in
  msys_nt* | cygwin_nt* | mingw*) platform="windows" ;;
  linux) platform="linux" ;;
  darwin) platform="darwin" ;;
  *)
    error "Unsupported platform: ${platform}"
    exit 1
    ;;
  esac

  printf '%s' "${platform}"
}

# Currently supporting:
#   - x86_64
#   - arm64
#   - 386
#   - armv6
detect_arch() {
  arch="$(uname -m | tr '[:upper:]' '[:lower:]')"

  case "${arch}" in
  x86_64 | amd64) arch="amd64" ;;
  arm64 | aarch64) arch="arm64" ;;
  i386 | i686) arch="386" ;;
  armv6* | armv7*) arch="armv6" ;;
  *)
    error "Unsupported architecture: ${arch}"
    exit 1
    ;;
  esac

  printf '%s' "${arch}"
}

detect_target() {
  arch="$1"
  platform="$2"

  case "${platform}-${arch}" in
  linux-386) target="linux_386" ;;
  linux-amd64) target="linux_amd64" ;;
  linux-arm64) target="linux_arm64" ;;
  linux-armv6) target="linux_armv6" ;;
  darwin-amd64) target="darwin_amd64" ;;
  darwin-arm64) target="darwin_arm64" ;;
  windows-amd64) target="windows_amd64" ;;
  windows-386) target="windows_386" ;;
  *)
    error "Unsupported platform-architecture combination: ${platform}-${arch}"
    exit 1
    ;;
  esac

  printf '%s' "${target}"
}

get_latest_release() {
  if has curl; then
    curl --fail --silent "https://api.github.com/repos/adhocore/gronx/releases/latest" | sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p'
  elif has wget; then
    wget --quiet -O - "https://api.github.com/repos/adhocore/gronx/releases/latest" | sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p'
  else
    error "No HTTP download program (curl, wget) found, unable to fetch latest release"
    exit 1
  fi
}

url_encode() {
  # URL-encode a string
  # shellcheck disable=SC1003
  echo "$1" | sed 's/ /%20/g; s/"/%22/g; s/#/%23/g; s/%/%25/g; s/+/%2B/g'
}

confirm() {
  if [ -z "${FORCE-}" ]; then
    printf "%s " "${MAGENTA}?${NO_COLOR} $* ${BOLD}[y/N]${NO_COLOR}"
    set +e
    read -r yn </dev/tty
    rc=$?
    set -e
    if [ $rc -ne 0 ]; then
      error "Error reading from prompt (please re-run with the '--yes' option)"
      exit 1
    fi
    yn=$(echo "$yn" | tr '[:upper:]' '[:lower:]')
    case "$yn" in
      y* | j*) ;;
      *)
        error 'Aborting (please answer "yes" to continue)'
        exit 1
        ;;
    esac
  fi
}

check_bin_dir() {
  bin_dir="${1%/}"

  if [ ! -d "$bin_dir" ]; then
    error "Installation location $bin_dir does not appear to be a directory"
    info "Make sure the location exists and is a directory, then try again."
    usage
    exit 1
  fi

  # https://stackoverflow.com/a/11655875
  good=$(
    IFS=:
    for path in $PATH; do
      if [ "${path%/}" = "${bin_dir}" ]; then
        printf 1
        break
      fi
    done
  )

  if [ "${good}" != "1" ]; then
    warn "Bin directory ${bin_dir} is not in your \$PATH"
  fi
}

is_build_available() {
  target="$1"

  good=$(
    IFS=" "
    for t in $SUPPORTED_TARGETS; do
      if [ "${t}" = "${target}" ]; then
        printf 1
        break
      fi
    done
  )

  if [ "${good}" != "1" ]; then
    error "Build for ${target} is not yet available for tasker"
    printf "\n" >&2
    info "If you would like to see a build for your configuration,"
    info "please create an issue requesting a build for ${MAGENTA}${target}${NO_COLOR}:"
    info "${BOLD}${UNDERLINE}https://github.com/adhocore/gronx/issues/new/${NO_COLOR}"
    printf "\n"
    exit 1
  fi
}

# defaults
if [ -z "${PLATFORM-}" ]; then
  PLATFORM="$(detect_platform)"
fi

if [ -z "${BIN_DIR-}" ]; then
  BIN_DIR="/usr/local/bin"
fi

if [ -z "${ARCH-}" ]; then
  ARCH="$(detect_arch)"
fi

if [ -z "${BASE_URL-}" ]; then
  BASE_URL="https://github.com/adhocore/gronx/releases"
fi

if [ -z "${VERSION-}" ]; then
  VERSION="latest"
fi

# Non-POSIX shells can break once executing code due to semantic differences
verify_shell_is_posix_or_exit

# parse argv variables
while [ "$#" -gt 0 ]; do
  case "$1" in
  -p | --platform)
    PLATFORM="$2"
    shift 2
    ;;
  -b | --bin-dir)
    BIN_DIR="$2"
    shift 2
    ;;
  -a | --arch)
    ARCH="$2"
    shift 2
    ;;
  -B | --base-url)
    BASE_URL="$2"
    shift 2
    ;;
  -v | --version)
    VERSION="$2"
    shift 2
    ;;

  -V | --verbose)
    VERBOSE=1
    shift 1
    ;;
  -f | -y | --force | --yes)
    FORCE=1
    shift 1
    ;;
  -h | --help)
    usage
    exit
    ;;

  -p=* | --platform=*)
    PLATFORM="${1#*=}"
    shift 1
    ;;
  -b=* | --bin-dir=*)
    BIN_DIR="${1#*=}"
    shift 1
    ;;
  -a=* | --arch=*)
    ARCH="${1#*=}"
    shift 1
    ;;
  -B=* | --base-url=*)
    BASE_URL="${1#*=}"
    shift 1
    ;;
  -v=* | --version=*)
    VERSION="${1#*=}"
    shift 1
    ;;
  -V=* | --verbose=*)
    VERBOSE="${1#*=}"
    shift 1
    ;;
  -f=* | -y=* | --force=* | --yes=*)
    FORCE="${1#*=}"
    shift 1
    ;;

  *)
    error "Unknown option: $1"
    usage
    exit 1
    ;;
  esac
done

TARGET="$(detect_target "${ARCH}" "${PLATFORM}")"

is_build_available "${TARGET}"

if [ -n "${VERBOSE-}" ]; then
  VERBOSE=v
  info "${BOLD}Verbose${NO_COLOR}: yes"
else
  VERBOSE=
fi

printf "  %s\n" "${UNDERLINE}Configuration${NO_COLOR}"
info "${BOLD}Bin directory${NO_COLOR}: ${GREEN}${BIN_DIR}${NO_COLOR}"
info "${BOLD}Platform${NO_COLOR}:      ${GREEN}${PLATFORM}${NO_COLOR}"
info "${BOLD}Arch${NO_COLOR}:          ${GREEN}${ARCH}${NO_COLOR}"

# Get latest version if required
if [ "${VERSION}" = "latest" ]; then
  info "Fetching latest version..."
  VERSION="$(get_latest_release)"
  info "Latest version is ${VERSION}"
fi

# Strip the 'v' prefix from version if present
VERSION_NUMBER="${VERSION#v}"

# Determine archive extension based on platform
if [ "${PLATFORM}" = "windows" ]; then
  ARCHIVE_EXT="zip"
else
  ARCHIVE_EXT="tar.gz"
fi

ARCHIVE_FILE="tasker_${VERSION_NUMBER}_${TARGET}.${ARCHIVE_EXT}"

# URL-encode the VERSION
VERSION_ENCODED="$(url_encode "${VERSION}")"

printf '\n'

URL="${BASE_URL}/download/${VERSION_ENCODED}/${ARCHIVE_FILE}"

info "Download URL: ${UNDERLINE}${BLUE}${URL}${NO_COLOR}"
confirm "Install tasker ${GREEN}${VERSION}${NO_COLOR} to ${BOLD}${GREEN}${BIN_DIR}${NO_COLOR}?"
check_bin_dir "${BIN_DIR}"

install "${BIN_DIR}"
completed "tasker ${VERSION} installed"

printf '\n'
info "tasker has been installed to ${BIN_DIR}"
info "You can run it by typing 'tasker' in your terminal"
