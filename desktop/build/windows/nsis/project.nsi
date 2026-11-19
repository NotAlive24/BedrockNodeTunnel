Unicode true
RequestExecutionLevel user

!define PRODUCT_NAME "Nethernet Desktop"
!define PRODUCT_VERSION "1.0.0"
!define PRODUCT_EXE "NethernetDesktop.exe"
!ifndef ARG_WAILS_AMD64_BINARY
  !error "ARG_WAILS_AMD64_BINARY must point to the built executable"
!endif

VIProductVersion "1.0.0.0"
VIAddVersionKey "CompanyName" "NotAlive24"
VIAddVersionKey "FileDescription" "Nethernet Desktop Installer"
VIAddVersionKey "ProductVersion" "${PRODUCT_VERSION}"
VIAddVersionKey "ProductName" "${PRODUCT_NAME}"

!include "MUI.nsh"
Name "${PRODUCT_NAME}"
OutFile "..\\..\\..\\bin\\NethernetDesktop-installer.exe"
InstallDir "$LOCALAPPDATA\\Programs\\Nethernet Desktop"
ShowInstDetails show
!define MUI_FINISHPAGE_RUN "$INSTDIR\\${PRODUCT_EXE}"
!define MUI_FINISHPAGE_RUN_TEXT "Launch Nethernet Desktop"
!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH
!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES
!insertmacro MUI_LANGUAGE "English"

Section "Install"
  SetOutPath "$INSTDIR"
  File /oname=${PRODUCT_EXE} "${ARG_WAILS_AMD64_BINARY}"
  WriteUninstaller "$INSTDIR\\Uninstall.exe"
  CreateDirectory "$SMPROGRAMS\\Nethernet Desktop"
  CreateShortcut "$SMPROGRAMS\\Nethernet Desktop\\Nethernet Desktop.lnk" "$INSTDIR\\${PRODUCT_EXE}"
  CreateShortcut "$DESKTOP\\Nethernet Desktop.lnk" "$INSTDIR\\${PRODUCT_EXE}"
SectionEnd

Section "Uninstall"
  Delete "$SMPROGRAMS\\Nethernet Desktop\\Nethernet Desktop.lnk"
  RMDir "$SMPROGRAMS\\Nethernet Desktop"
  Delete "$DESKTOP\\Nethernet Desktop.lnk"
  Delete "$INSTDIR\\${PRODUCT_EXE}"
  Delete "$INSTDIR\\Uninstall.exe"
  RMDir "$INSTDIR"
SectionEnd
