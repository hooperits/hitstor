; HitStor Server Installer for Windows
; Build with: makensis -DVERSION=x.y.z -DBIN_DIR=../../bin packaging/nsis/hitstor-server.nsi

!include "MUI2.nsh"
!include "EnvVarUpdate.nsh"
!include "FileFunc.nsh"

; --- Configuration ---
!ifndef VERSION
  !define VERSION "dev"
!endif
!ifndef BIN_DIR
  !define BIN_DIR "../../bin"
!endif

Name "HitStor Server ${VERSION}"
OutFile "../../dist/hitstor-${VERSION}-windows-amd64-setup.exe"
InstallDir "$PROGRAMFILES\HitStor"
InstallDirRegKey HKLM "Software\HitStor" "InstallDir"
RequestExecutionLevel admin

; --- MUI Settings ---
!define MUI_ICON "hitstor.ico"
!define MUI_UNICON "hitstor.ico"
!define MUI_ABORTWARNING

; --- Pages ---
!insertmacro MUI_PAGE_LICENSE "license.txt"
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH

!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES

!insertmacro MUI_LANGUAGE "English"
!insertmacro GetSize

; --- Install Section ---
Section "HitStor Server" SecMain
  SectionIn RO

  SetOutPath "$INSTDIR"
  File "/oname=hitstor.exe" "${BIN_DIR}/hitstor.exe"
  File "hitstor.ico"

  ; Create config directory.
  CreateDirectory "$COMMONAPPDATA\HitStor"
  SetOutPath "$COMMONAPPDATA\HitStor"
  File "/oname=config.yaml.example" "../config/server-config.yaml.example"

  ; Add to PATH.
  ${EnvVarUpdate} $0 "PATH" "A" "HKLM" "$INSTDIR"

  ; Start Menu shortcuts.
  CreateDirectory "$SMPROGRAMS\HitStor"
  CreateShortcut "$SMPROGRAMS\HitStor\Uninstall HitStor.lnk" "$INSTDIR\uninstall.exe"

  ; Registry for Add/Remove Programs.
  WriteRegStr HKLM "Software\HitStor" "InstallDir" "$INSTDIR"
  WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\HitStor" \
    "DisplayName" "HitStor Server"
  WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\HitStor" \
    "DisplayVersion" "${VERSION}"
  WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\HitStor" \
    "UninstallString" "$\"$INSTDIR\uninstall.exe$\""
  WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\HitStor" \
    "DisplayIcon" "$INSTDIR\hitstor.ico"
  WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\HitStor" \
    "Publisher" "HOOPER IT SERVICES"
  WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\HitStor" \
    "InstallLocation" "$INSTDIR"
  WriteRegDWORD HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\HitStor" \
    "NoModify" 1
  WriteRegDWORD HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\HitStor" \
    "NoRepair" 1

  ; Create uninstaller.
  WriteUninstaller "$INSTDIR\uninstall.exe"
SectionEnd

Function UpdateARPInstallSize
  Push $R0
  Push $R1
  Push $R2
  Push $R3

  StrCpy $R0 0
  ${GetSize} "$INSTDIR" "/S=0K" $R1 $R2 $R3
  IntOp $R0 $R0 + $R1

  IfFileExists "$COMMONAPPDATA\HitStor\*.*" 0 +3
    ${GetSize} "$COMMONAPPDATA\HitStor" "/S=0K" $R1 $R2 $R3
    IntOp $R0 $R0 + $R1

  WriteRegDWORD HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\HitStor" \
    "EstimatedSize" $R0

  Pop $R3
  Pop $R2
  Pop $R1
  Pop $R0
FunctionEnd

Function .onInstSuccess
  Call UpdateARPInstallSize
FunctionEnd

; --- Uninstall Section ---
Section "Uninstall"
  Delete "$INSTDIR\hitstor.exe"
  Delete "$INSTDIR\hitstor.ico"
  Delete "$INSTDIR\uninstall.exe"
  RMDir "$INSTDIR"

  ; Remove from PATH.
  ${un.EnvVarUpdate} $0 "PATH" "R" "HKLM" "$INSTDIR"

  ; Remove Start Menu shortcuts.
  Delete "$SMPROGRAMS\HitStor\Uninstall HitStor.lnk"
  RMDir "$SMPROGRAMS\HitStor"

  ; Remove registry entries.
  DeleteRegKey HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\HitStor"
  DeleteRegKey HKLM "Software\HitStor"
SectionEnd
