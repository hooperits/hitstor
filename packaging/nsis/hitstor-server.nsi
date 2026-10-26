; HitStor Server Installer for Windows
; Build with: makensis -DVERSION=x.y.z -DBIN_DIR=../../bin packaging/nsis/hitstor-server.nsi
;
; Supports unattended install/upgrade: hitstor-x.y.z-windows-amd64-setup.exe /S
; (all sections run, including Windows Service and firewall rules).

!include "MUI2.nsh"
!include "LogicLib.nsh"
!include "FileFunc.nsh"
!include "WordFunc.nsh"
!include "WinMessages.nsh"

; --- Configuration ---
!ifndef VERSION
  !define VERSION "dev"
!endif
!ifndef BIN_DIR
  !define BIN_DIR "../../bin"
!endif

!define SVC_NAME "HitStor"
!define GRPC_PORT "8420"
!define DASHBOARD_PORT "8421"
!define ENV_KEY "SYSTEM\CurrentControlSet\Control\Session Manager\Environment"
!define ARP_KEY "Software\Microsoft\Windows\CurrentVersion\Uninstall\HitStor"

Name "HitStor Server ${VERSION}"
OutFile "../../dist/hitstor-${VERSION}-windows-amd64-setup.exe"
InstallDir "$PROGRAMFILES64\HitStor"
InstallDirRegKey HKLM "Software\HitStor" "InstallDir"
RequestExecutionLevel admin

; --- Variables ---
Var ConfigDir

; --- MUI Settings ---
!define MUI_ICON "hitstor.ico"
!define MUI_UNICON "hitstor.ico"
!define MUI_ABORTWARNING

; --- Pages ---
!insertmacro MUI_PAGE_LICENSE "license.txt"
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_COMPONENTS
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH

!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES

!insertmacro MUI_LANGUAGE "English"
!insertmacro GetSize

; ============================================================================
; PATH Helper Functions (replace EnvVarUpdate dependency)
; ============================================================================

; AddToPath - Appends $INSTDIR to the system PATH if not already present.
; Entries are compared whole (";entry;") so "C:\...\HitStor" does not match
; "C:\...\HitStor Agent".
Function AddToPath
  ReadRegStr $0 HKLM "${ENV_KEY}" "Path"
  ; A PATH longer than NSIS strings is read truncated (NSIS_MAX_STRLEN - 1)
  ; and would be cut short on write-back.
  StrLen $1 $0
  IntOp $2 ${NSIS_MAX_STRLEN} - 1
  ${If} $1 >= $2
    DetailPrint "Warning: system PATH too long to edit safely; add $INSTDIR manually."
    Return
  ${EndIf}
  ${If} $0 == ""
    WriteRegExpandStr HKLM "${ENV_KEY}" "Path" "$INSTDIR"
  ${Else}
    ${WordFind} ";$0;" ";$INSTDIR;" "E+1{" $1
    ${IfNot} ${Errors}
      Return ; already in PATH
    ${EndIf}
    WriteRegExpandStr HKLM "${ENV_KEY}" "Path" "$0;$INSTDIR"
  ${EndIf}
  ; Notify running processes of the change.
  SendMessage ${HWND_BROADCAST} ${WM_WININICHANGE} 0 "STR:Environment" /TIMEOUT=5000
FunctionEnd

; un.RemoveFromPath - Removes the $INSTDIR entry from the system PATH.
Function un.RemoveFromPath
  ReadRegStr $0 HKLM "${ENV_KEY}" "Path"
  StrLen $1 $0
  IntOp $2 ${NSIS_MAX_STRLEN} - 1
  ${If} $0 == ""
  ${OrIf} $1 >= $2
    Return
  ${EndIf}
  ${WordReplace} ";$0;" ";$INSTDIR;" ";" "+*" $1
  ; Strip the padding semicolons added above.
  StrCpy $1 $1 -1
  StrCpy $1 $1 "" 1
  ${If} $1 != $0
    WriteRegExpandStr HKLM "${ENV_KEY}" "Path" "$1"
    SendMessage ${HWND_BROADCAST} ${WM_WININICHANGE} 0 "STR:Environment" /TIMEOUT=5000
  ${EndIf}
FunctionEnd

; ============================================================================
; Initialization
; ============================================================================

Function .onInit
  ; $COMMONAPPDATA is not available when cross-compiled on Linux.
  ; Use %ProgramData% env var instead (always set on Vista+).
  ReadEnvStr $0 "ProgramData"
  ${If} $0 == ""
    StrCpy $0 "C:\ProgramData"
  ${EndIf}
  StrCpy $ConfigDir "$0\HitStor"
FunctionEnd

Function un.onInit
  ReadEnvStr $0 "ProgramData"
  ${If} $0 == ""
    StrCpy $0 "C:\ProgramData"
  ${EndIf}
  StrCpy $ConfigDir "$0\HitStor"
FunctionEnd

; ============================================================================
; Required Section: Server Binary + Config
; ============================================================================

Section "HitStor Server" SecMain
  SectionIn RO

  ; --- Upgrade: stop existing service before replacing binary ---
  ${If} ${FileExists} "$INSTDIR\hitstor.exe"
    DetailPrint "Stopping existing service..."
    nsExec::ExecToLog '"$INSTDIR\hitstor.exe" service stop --name ${SVC_NAME}'
    Pop $0
    Sleep 2000
  ${EndIf}

  ; Install binary.
  SetOutPath "$INSTDIR"
  File "/oname=hitstor.exe" "${BIN_DIR}/hitstor.exe"
  File "hitstor.ico"

  ; Config directory holds config.yaml (dashboard password hash, encrypted
  ; credentials) and server logs: restrict it to Administrators and SYSTEM.
  CreateDirectory "$ConfigDir"
  nsExec::ExecToLog 'icacls "$ConfigDir" /inheritance:r /grant:r *S-1-5-32-544:(OI)(CI)F /grant:r *S-1-5-18:(OI)(CI)F'
  Pop $0
  ${If} $0 != 0
    DetailPrint "Warning: could not restrict permissions on $ConfigDir (exit code $0)"
  ${EndIf}

  SetOutPath "$ConfigDir"
  File "/oname=config.yaml.example" "../config/server-config.yaml.example"

  ; Create config.yaml if missing (preserve on upgrade). It must exist so the
  ; server can persist settings made from the dashboard, such as the
  ; generated dashboard password.
  ${IfNot} ${FileExists} "$ConfigDir\config.yaml"
    CopyFiles /SILENT "$ConfigDir\config.yaml.example" "$ConfigDir\config.yaml"
    DetailPrint "Created $ConfigDir\config.yaml"
  ${Else}
    DetailPrint "Existing configuration preserved."
  ${EndIf}

  ; Add to PATH.
  Call AddToPath

  ; Start Menu shortcuts.
  CreateDirectory "$SMPROGRAMS\HitStor"
  CreateShortcut "$SMPROGRAMS\HitStor\Uninstall HitStor.lnk" "$INSTDIR\uninstall.exe"

  ; Registry for Add/Remove Programs.
  WriteRegStr HKLM "Software\HitStor" "InstallDir" "$INSTDIR"
  WriteRegStr HKLM "${ARP_KEY}" "DisplayName" "HitStor Server"
  WriteRegStr HKLM "${ARP_KEY}" "DisplayVersion" "${VERSION}"
  WriteRegStr HKLM "${ARP_KEY}" "UninstallString" "$\"$INSTDIR\uninstall.exe$\""
  WriteRegStr HKLM "${ARP_KEY}" "QuietUninstallString" "$\"$INSTDIR\uninstall.exe$\" /S"
  WriteRegStr HKLM "${ARP_KEY}" "DisplayIcon" "$INSTDIR\hitstor.ico"
  WriteRegStr HKLM "${ARP_KEY}" "Publisher" "HOOPER IT SERVICES"
  WriteRegStr HKLM "${ARP_KEY}" "InstallLocation" "$INSTDIR"
  WriteRegDWORD HKLM "${ARP_KEY}" "NoModify" 1
  WriteRegDWORD HKLM "${ARP_KEY}" "NoRepair" 1

  ; Create uninstaller.
  WriteUninstaller "$INSTDIR\uninstall.exe"
SectionEnd

; ============================================================================
; Section: Windows Firewall rules (default ON)
; ============================================================================

Section "Windows Firewall rules" SecFirewall
  DetailPrint "Adding Windows Firewall rules for ports ${GRPC_PORT} and ${DASHBOARD_PORT}..."
  nsExec::ExecToLog 'netsh advfirewall firewall delete rule name="HitStor Server gRPC"'
  Pop $0
  nsExec::ExecToLog 'netsh advfirewall firewall delete rule name="HitStor Server Dashboard"'
  Pop $0
  nsExec::ExecToLog 'netsh advfirewall firewall add rule name="HitStor Server gRPC" dir=in action=allow protocol=TCP localport=${GRPC_PORT} program="$INSTDIR\hitstor.exe"'
  Pop $0
  nsExec::ExecToLog 'netsh advfirewall firewall add rule name="HitStor Server Dashboard" dir=in action=allow protocol=TCP localport=${DASHBOARD_PORT} program="$INSTDIR\hitstor.exe"'
  Pop $0
SectionEnd

; ============================================================================
; Section: Install as Windows Service (default ON)
; ============================================================================

Section "Install as Windows Service" SecService
  DetailPrint "Installing Windows Service..."

  ; Re-create the service definition so it picks up the current binary path
  ; and arguments. Fails harmlessly on a fresh install.
  nsExec::ExecToLog '"$INSTDIR\hitstor.exe" service uninstall --name ${SVC_NAME}'
  Pop $0
  Sleep 1000

  nsExec::ExecToLog '"$INSTDIR\hitstor.exe" service install --name ${SVC_NAME} --config "$ConfigDir\config.yaml"'
  Pop $0
  ${If} $0 != 0
    DetailPrint "Warning: service install returned exit code $0"
  ${EndIf}

  DetailPrint "Starting HitStor service..."
  nsExec::ExecToLog '"$INSTDIR\hitstor.exe" service start --name ${SVC_NAME}'
  Pop $0
  ${If} $0 != 0
    DetailPrint "Warning: service start returned exit code $0 (see $ConfigDir\logs\server.log)"
  ${EndIf}
SectionEnd

; --- Section descriptions ---
!insertmacro MUI_FUNCTION_DESCRIPTION_BEGIN
  !insertmacro MUI_DESCRIPTION_TEXT ${SecMain} "Install the HitStor server binary and configuration."
  !insertmacro MUI_DESCRIPTION_TEXT ${SecFirewall} "Allow inbound TCP ${GRPC_PORT} (agents) and ${DASHBOARD_PORT} (dashboard) for hitstor.exe."
  !insertmacro MUI_DESCRIPTION_TEXT ${SecService} "Register and start HitStor as a Windows Service (hitstor serve)."
!insertmacro MUI_FUNCTION_DESCRIPTION_END

; ============================================================================
; ARP size (Programs and Features "Size" column)
; ============================================================================

; EstimatedSize is stored in kilobytes (DWORD).
Function UpdateARPInstallSize
  Push $R0
  Push $R1
  Push $R2
  Push $R3

  StrCpy $R0 0
  ${GetSize} "$INSTDIR" "/S=0K" $R1 $R2 $R3
  IntOp $R0 $R0 + $R1

  WriteRegDWORD HKLM "${ARP_KEY}" "EstimatedSize" $R0

  Pop $R3
  Pop $R2
  Pop $R1
  Pop $R0
FunctionEnd

Function .onInstSuccess
  Call UpdateARPInstallSize
FunctionEnd

; ============================================================================
; Uninstall Section
; ============================================================================

Section "Uninstall"
  ; Stop and remove Windows Service.
  nsExec::ExecToLog '"$INSTDIR\hitstor.exe" service stop --name ${SVC_NAME}'
  Pop $0
  Sleep 2000
  nsExec::ExecToLog '"$INSTDIR\hitstor.exe" service uninstall --name ${SVC_NAME}'
  Pop $0

  ; Remove firewall rules.
  nsExec::ExecToLog 'netsh advfirewall firewall delete rule name="HitStor Server gRPC"'
  Pop $0
  nsExec::ExecToLog 'netsh advfirewall firewall delete rule name="HitStor Server Dashboard"'
  Pop $0

  Delete "$INSTDIR\hitstor.exe"
  Delete "$INSTDIR\hitstor.ico"
  Delete "$INSTDIR\uninstall.exe"
  RMDir "$INSTDIR"

  ; Keep config.yaml and logs: they reference repositories and encrypted
  ; credentials the operator may still need.
  Delete "$ConfigDir\config.yaml.example"
  DetailPrint "Configuration kept in $ConfigDir"

  ; Remove from PATH.
  Call un.RemoveFromPath

  ; Remove Start Menu shortcuts.
  Delete "$SMPROGRAMS\HitStor\Uninstall HitStor.lnk"
  RMDir "$SMPROGRAMS\HitStor"

  ; Remove registry entries.
  DeleteRegKey HKLM "${ARP_KEY}"
  DeleteRegKey HKLM "Software\HitStor"
SectionEnd
