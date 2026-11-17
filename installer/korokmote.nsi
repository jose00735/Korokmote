; Korokmote Windows installer (NSIS 3, native 64-bit).
; Build: makensis -DVERSION=1.0.0 -DBINDIR=..\dist\windows-amd64 korokmote.nsi

Target amd64-unicode
SetCompressor /SOLID lzma

!ifndef VERSION
  !define VERSION "0.0.0"
!endif
!ifndef BINDIR
  !define BINDIR "..\dist\windows-amd64"
!endif
!ifndef OUTDIR
  !define OUTDIR "..\dist"
!endif

!define APPNAME "Korokmote"
!define EXE "korokmoted.exe"
!define UNINSTKEY "Software\Microsoft\Windows\CurrentVersion\Uninstall\Korokmote"
!define FWRULE "Korokmote"

Name "${APPNAME}"
OutFile "${OUTDIR}\korokmote-setup-${VERSION}-x64.exe"
InstallDir "$PROGRAMFILES64\Korokmote"
InstallDirRegKey HKLM "Software\Korokmote" "InstallDir"
RequestExecutionLevel admin
BrandingText "Korokmote ${VERSION} :: open source"
VIProductVersion "${VERSION}.0"
VIAddVersionKey "ProductName" "Korokmote"
VIAddVersionKey "FileDescription" "Korokmote installer"
VIAddVersionKey "FileVersion" "${VERSION}"
VIAddVersionKey "ProductVersion" "${VERSION}"
VIAddVersionKey "LegalCopyright" "MIT License"

!include "MUI2.nsh"
!include "x64.nsh"
!include "LogicLib.nsh"
!include "FileFunc.nsh"

!define MUI_ICON "korokmote.ico"
!define MUI_UNICON "korokmote.ico"
!define MUI_WELCOMEFINISHPAGE_BITMAP "welcome.bmp"
!define MUI_UNWELCOMEFINISHPAGE_BITMAP "welcome.bmp"
!define MUI_HEADERIMAGE
!define MUI_HEADERIMAGE_RIGHT
!define MUI_HEADERIMAGE_BITMAP "header.bmp"
!define MUI_ABORTWARNING
!define MUI_COMPONENTSPAGE_NODESC

!define MUI_WELCOMEPAGE_TITLE "Welcome to Korokmote ${VERSION}"
!define MUI_WELCOMEPAGE_TEXT "Korokmote lets your Android phone control this PC over your home network: touchpad, keyboard, media, volume, power, apps and files.$\r$\n$\r$\nNo account, no cloud, no subscription. Everything stays on your network.$\r$\n$\r$\nClick Next to continue."
!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_LICENSE "LICENSE.txt"
!insertmacro MUI_PAGE_COMPONENTS
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES
!define MUI_FINISHPAGE_TITLE "Korokmote is installed"
!define MUI_FINISHPAGE_TEXT "Korokmote runs in the notification area (look for the green K).$\r$\n$\r$\nTo connect your phone: install the Korokmote app, tap [ + PAIR ] and scan the code shown on the pairing page.$\r$\n$\r$\nIf the phone cannot find this PC, make sure your Wi-Fi is set to a Private network in Windows settings."
!define MUI_FINISHPAGE_RUN
!define MUI_FINISHPAGE_RUN_TEXT "Start Korokmote and pair a phone now"
!define MUI_FINISHPAGE_RUN_FUNCTION LaunchAsUser
!insertmacro MUI_PAGE_FINISH

!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_COMPONENTS
!insertmacro MUI_UNPAGE_INSTFILES

!insertmacro MUI_LANGUAGE "English"
!insertmacro MUI_LANGUAGE "Spanish"
!insertmacro MUI_LANGUAGE "PortugueseBR"

Function .onInit
  ${IfNot} ${RunningX64}
    MessageBox MB_ICONSTOP "Korokmote requires 64-bit Windows."
    Abort
  ${EndIf}
  SetRegView 64
  SetShellVarContext all
FunctionEnd

Function un.onInit
  SetRegView 64
  SetShellVarContext all
FunctionEnd

; Launch through Explorer so the daemon runs un-elevated, as the signed-in user.
Function LaunchAsUser
  Exec '"$WINDIR\explorer.exe" "$SMPROGRAMS\Korokmote\Pair a phone.lnk"'
FunctionEnd

!macro StopDaemon
  nsExec::Exec '"$SYSDIR\taskkill.exe" /F /IM ${EXE}'
  Pop $0
  Sleep 500
!macroend

Section "Korokmote (required)" SecCore
  SectionIn RO
  !insertmacro StopDaemon
  SetOutPath "$INSTDIR"
  File "${BINDIR}\korokmoted.exe"
  File "${BINDIR}\korokctl.exe"
  File "korokmote.ico"
  File /oname=LICENSE.txt "LICENSE.txt"
  WriteUninstaller "$INSTDIR\uninstall.exe"

  CreateDirectory "$SMPROGRAMS\Korokmote"
  CreateShortcut "$SMPROGRAMS\Korokmote\Korokmote.lnk" "$INSTDIR\korokmoted.exe" "" "$INSTDIR\korokmote.ico"
  CreateShortcut "$SMPROGRAMS\Korokmote\Pair a phone.lnk" "$INSTDIR\korokmoted.exe" "--pair" "$INSTDIR\korokmote.ico"
  CreateShortcut "$SMPROGRAMS\Korokmote\Uninstall Korokmote.lnk" "$INSTDIR\uninstall.exe"

  WriteRegStr HKLM "Software\Korokmote" "InstallDir" "$INSTDIR"
  WriteRegStr HKLM "${UNINSTKEY}" "DisplayName" "Korokmote"
  WriteRegStr HKLM "${UNINSTKEY}" "DisplayVersion" "${VERSION}"
  WriteRegStr HKLM "${UNINSTKEY}" "Publisher" "Korokmote contributors"
  WriteRegStr HKLM "${UNINSTKEY}" "URLInfoAbout" "https://github.com/jose00735/Korokmote"
  WriteRegStr HKLM "${UNINSTKEY}" "DisplayIcon" "$INSTDIR\korokmote.ico"
  WriteRegStr HKLM "${UNINSTKEY}" "InstallLocation" "$INSTDIR"
  WriteRegStr HKLM "${UNINSTKEY}" "UninstallString" '"$INSTDIR\uninstall.exe"'
  WriteRegStr HKLM "${UNINSTKEY}" "QuietUninstallString" '"$INSTDIR\uninstall.exe" /S'
  WriteRegDWORD HKLM "${UNINSTKEY}" "NoModify" 1
  WriteRegDWORD HKLM "${UNINSTKEY}" "NoRepair" 1
  ${GetSize} "$INSTDIR" "/S=0K" $0 $1 $2
  IntFmt $0 "0x%08X" $0
  WriteRegDWORD HKLM "${UNINSTKEY}" "EstimatedSize" "$0"

  ; Firewall: allow the daemon (TCP 47810, UDP 47811 discovery, UDP 5353 mDNS) on private networks.
  nsExec::Exec 'netsh advfirewall firewall delete rule name="${FWRULE}"'
  Pop $0
  nsExec::Exec 'netsh advfirewall firewall add rule name="${FWRULE}" dir=in action=allow program="$INSTDIR\korokmoted.exe" enable=yes profile=private,domain description="Korokmote remote control"'
  Pop $0
SectionEnd

Section "Start with Windows" SecAutostart
  ; Written for the signed-in user by the daemon itself on its first run.
  WriteRegStr HKLM "Software\Korokmote" "Autostart" "user"
SectionEnd

Section /o "Also allow on Public networks (not recommended)" SecPublic
  nsExec::Exec 'netsh advfirewall firewall add rule name="${FWRULE}" dir=in action=allow program="$INSTDIR\korokmoted.exe" enable=yes profile=public description="Korokmote remote control (public networks)"'
  Pop $0
SectionEnd

Section "-post"
  ${IfNot} ${SectionIsSelected} ${SecAutostart}
    WriteRegStr HKLM "Software\Korokmote" "Autostart" "off"
  ${EndIf}
SectionEnd

Section "un.Korokmote" UnSecCore
  SectionIn RO
  !insertmacro StopDaemon
  nsExec::Exec 'netsh advfirewall firewall delete rule name="${FWRULE}"'
  Pop $0
  DeleteRegValue HKCU "Software\Microsoft\Windows\CurrentVersion\Run" "Korokmote"
  Delete "$INSTDIR\korokmoted.exe"
  Delete "$INSTDIR\korokctl.exe"
  Delete "$INSTDIR\korokmote.ico"
  Delete "$INSTDIR\LICENSE.txt"
  Delete "$INSTDIR\uninstall.exe"
  RMDir "$INSTDIR"
  Delete "$SMPROGRAMS\Korokmote\*.lnk"
  RMDir "$SMPROGRAMS\Korokmote"
  DeleteRegKey HKLM "${UNINSTKEY}"
  DeleteRegKey HKLM "Software\Korokmote"
SectionEnd

Section /o "un.Remove my settings and paired devices" UnSecData
  RMDir /r "$APPDATA\Korokmote"
  RMDir /r "$LOCALAPPDATA\Korokmote"
SectionEnd
