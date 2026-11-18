Unicode true

!include "wails_tools.nsh"

VIProductVersion "1.0.0.0"
VIFileVersion "1.0.0.0"
VIAddVersionKey "CompanyName" "NotAlive24"
VIAddVersionKey "FileDescription" "Nethernet Desktop Installer"
VIAddVersionKey "ProductVersion" "1.0.0"
VIAddVersionKey "FileVersion" "1.0.0"
VIAddVersionKey "LegalCopyright" "Copyright 2026 NotAlive24"
VIAddVersionKey "ProductName" "Nethernet Desktop"

ManifestDPIAware true
!include "MUI.nsh"
!define MUI_ICON "..\icon.ico"
!define MUI_UNICON "..\icon.ico"
!define MUI_FINISHPAGE_NOAUTOCLOSE
!define MUI_ABORTWARNING

!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH
!insertmacro MUI_UNPAGE_INSTFILES
!insertmacro MUI_LANGUAGE "English"

Name "Nethernet Desktop"
OutFile "..\..\..\bin\NethernetDesktop-installer.exe"
InstallDir "$LOCALAPPDATA\Programs\Nethernet Desktop"
RequestExecutionLevel user
ShowInstDetails show

Function .onInit
  !insertmacro wails.checkArchitecture
FunctionEnd

Section "Install"
  !insertmacro wails.setShellContext
  !insertmacro wails.webview2runtime
  SetOutPath $INSTDIR
  !insertmacro wails.files
  CreateDirectory "$SMPROGRAMS\Nethernet Desktop"
  CreateShortcut "$SMPROGRAMS\Nethernet Desktop\Nethernet Desktop.lnk" "$INSTDIR\NethernetDesktop.exe"
  CreateShortcut "$DESKTOP\Nethernet Desktop.lnk" "$INSTDIR\NethernetDesktop.exe"
  !insertmacro wails.writeUninstaller
SectionEnd

Section "Uninstall"
  !insertmacro wails.setShellContext
  RMDir /r "$INSTDIR"
  RMDir /r "$SMPROGRAMS\Nethernet Desktop"
  Delete "$DESKTOP\Nethernet Desktop.lnk"
  !insertmacro wails.deleteUninstaller
SectionEnd
