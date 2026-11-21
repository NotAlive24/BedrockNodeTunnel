Unicode true
RequestExecutionLevel user

!define PRODUCT_NAME "Nethernet Desktop"
!ifndef PRODUCT_VERSION
!define PRODUCT_VERSION "2.0.0"
!endif
!define PRODUCT_EXE "NethernetDesktop.exe"
!ifndef ARG_DESKTOP_BINARY
  !error "ARG_DESKTOP_BINARY must point to the built executable"
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
  SetShellVarContext current
  SetOutPath "$INSTDIR"
  File /oname=${PRODUCT_EXE} "${ARG_DESKTOP_BINARY}"
  WriteUninstaller "$INSTDIR\\Uninstall.exe"
  CreateDirectory "$SMPROGRAMS\\Nethernet Desktop"
  CreateShortcut "$SMPROGRAMS\\Nethernet Desktop\\Nethernet Desktop.lnk" "$INSTDIR\\${PRODUCT_EXE}"
  CreateShortcut "$DESKTOP\\Nethernet Desktop.lnk" "$INSTDIR\\${PRODUCT_EXE}"
SectionEnd

Section "Uninstall"
  SetShellVarContext current
  Delete "$SMPROGRAMS\\Nethernet Desktop\\Nethernet Desktop.lnk"
  RMDir "$SMPROGRAMS\\Nethernet Desktop"
  Delete "$DESKTOP\\Nethernet Desktop.lnk"
  Delete "$INSTDIR\\${PRODUCT_EXE}"
  Delete "$INSTDIR\\Uninstall.exe"
  RMDir "$INSTDIR"
SectionEnd
