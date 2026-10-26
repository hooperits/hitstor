; HitStor Agent Installer for Windows
; Build with: makensis -DVERSION=x.y.z -DBIN_DIR=../../bin packaging/nsis/hitstor-agent.nsi

!include "MUI2.nsh"
!include "nsDialogs.nsh"
!include "LogicLib.nsh"
!include "FileFunc.nsh"
!include "TextFunc.nsh"
!include "WordFunc.nsh"
!include "WinMessages.nsh"

; --- Configuration ---
!ifndef VERSION
  !define VERSION "dev"
!endif
!ifndef BIN_DIR
  !define BIN_DIR "../../bin"
!endif

!define SVC_NAME "HitStorAgent"
!define ENV_KEY "SYSTEM\CurrentControlSet\Control\Session Manager\Environment"

Name "HitStor Agent ${VERSION}"
OutFile "../../dist/hitstor-agent-${VERSION}-windows-amd64-setup.exe"
InstallDir "$PROGRAMFILES\HitStor Agent"
InstallDirRegKey HKLM "Software\HitStorAgent" "InstallDir"
RequestExecutionLevel admin

; --- Variables ---
Var ConfigDir
Var ListenPort
Var ListenPortHWND
Var IsUpgrade
Var AgentToken

; --- MUI Settings ---
!define MUI_ICON "hitstor.ico"
!define MUI_UNICON "hitstor.ico"
!define MUI_HEADERIMAGE
!define MUI_HEADERIMAGE_BITMAP "header.bmp"
!define MUI_HEADERIMAGE_RIGHT
!define MUI_WELCOMEFINISHPAGE_BITMAP "welcome.bmp"
!define MUI_ABORTWARNING

; Disable the default Finish page (we use a custom post-install page).
!define MUI_FINISHPAGE_NOAUTOCLOSE

; --- Pages ---
!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_LICENSE "license.txt"
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_COMPONENTS
Page custom ListenPortPage ListenPortPageLeave
!insertmacro MUI_PAGE_INSTFILES
Page custom PostInstallPage

!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES

!insertmacro MUI_LANGUAGE "English"
!insertmacro GetSize

; ============================================================================
; PATH Helper Functions (replace EnvVarUpdate dependency)
; ============================================================================

; AddToPath - Appends $INSTDIR to the system PATH if not already present.
Function AddToPath
  ReadRegStr $0 HKLM "${ENV_KEY}" "Path"
  ${If} $0 == ""
    WriteRegExpandStr HKLM "${ENV_KEY}" "Path" "$INSTDIR"
  ${Else}
    ; Check if already in PATH.
    StrLen $1 "$INSTDIR"
    StrLen $2 $0
    ${If} $1 <= $2
      ; Simple substring check.
      ${WordFind} $0 "$INSTDIR" "E+1{" $3
      ${IfNot} ${Errors}
        Return ; already in PATH
      ${EndIf}
    ${EndIf}
    WriteRegExpandStr HKLM "${ENV_KEY}" "Path" "$0;$INSTDIR"
  ${EndIf}
  ; Notify running processes of the change.
  SendMessage ${HWND_BROADCAST} ${WM_WININICHANGE} 0 "STR:Environment" /TIMEOUT=5000
FunctionEnd

; un.RemoveFromPath - Removes $INSTDIR from the system PATH.
Function un.RemoveFromPath
  ReadRegStr $0 HKLM "${ENV_KEY}" "Path"
  ${If} $0 == ""
    Return
  ${EndIf}
  ; Remove ";$INSTDIR" or "$INSTDIR;" or exact match "$INSTDIR".
  ${WordReplace} $0 ";$INSTDIR" "" "+*" $1
  ${If} $1 != $0
    StrCpy $0 $1
  ${Else}
    ${WordReplace} $0 "$INSTDIR;" "" "+*" $1
    ${If} $1 != $0
      StrCpy $0 $1
    ${Else}
      ${WordReplace} $0 "$INSTDIR" "" "+*" $1
      StrCpy $0 $1
    ${EndIf}
  ${EndIf}
  WriteRegExpandStr HKLM "${ENV_KEY}" "Path" "$0"
  SendMessage ${HWND_BROADCAST} ${WM_WININICHANGE} 0 "STR:Environment" /TIMEOUT=5000
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

  StrCpy $ListenPort "8422"
  StrCpy $IsUpgrade "0"

  ; Detect existing installation (upgrade).
  IfFileExists "$ConfigDir\agent.env" 0 +2
    StrCpy $IsUpgrade "1"
FunctionEnd

Function un.onInit
  ReadEnvStr $0 "ProgramData"
  ${If} $0 == ""
    StrCpy $0 "C:\ProgramData"
  ${EndIf}
  StrCpy $ConfigDir "$0\HitStor"
FunctionEnd

; ============================================================================
; Custom Page: Listen Port Configuration
; ============================================================================

Function ListenPortPage
  !insertmacro MUI_HEADER_TEXT "Agent Configuration" "Configure the agent listen port for dashboard management."

  nsDialogs::Create 1018
  Pop $0

  ${NSD_CreateLabel} 0 0 100% 36u "The agent listens on a gRPC port so the HitStor server can manage it (registration, file browsing, backup triggers).$\r$\n$\r$\nLeave the default unless another service uses port 8422."
  Pop $0

  ${NSD_CreateLabel} 0 48u 80u 12u "Listen port:"
  Pop $0

  ${NSD_CreateText} 85u 46u 60u 14u $ListenPort
  Pop $ListenPortHWND

  ${If} $IsUpgrade == "1"
    ${NSD_CreateLabel} 0 72u 100% 12u "Existing installation detected. Credentials will be preserved."
    Pop $0
  ${EndIf}

  nsDialogs::Show
FunctionEnd

Function ListenPortPageLeave
  ${NSD_GetText} $ListenPortHWND $ListenPort

  ; Validate port is a number between 1 and 65535.
  ${If} $ListenPort == ""
    MessageBox MB_OK|MB_ICONEXCLAMATION "Please enter a port number."
    Abort
  ${EndIf}

  IntOp $0 $ListenPort + 0
  ${If} $0 < 1
  ${OrIf} $0 > 65535
    MessageBox MB_OK|MB_ICONEXCLAMATION "Port must be between 1 and 65535."
    Abort
  ${EndIf}
FunctionEnd

; ============================================================================
; Custom Page: Post-Install Summary
; ============================================================================

Function PostInstallPage
  !insertmacro MUI_HEADER_TEXT "Installation Complete" "The HitStor Agent is installed and running."

  nsDialogs::Create 1018
  Pop $0

  ; Try to retrieve the agent token.
  nsExec::ExecToStack '"$INSTDIR\hitstor-agent.exe" show-id --config "$ConfigDir\agent-config.yaml"'
  Pop $0  ; exit code
  Pop $AgentToken  ; stdout

  ${NSD_CreateLabel} 0 0 100% 12u "Agent is listening on port: $ListenPort"
  Pop $0

  ${NSD_CreateLabel} 0 18u 100% 12u "Agent Token:"
  Pop $0

  ${If} $AgentToken != ""
    ${NSD_CreateText} 0 32u 100% 14u $AgentToken
    Pop $0
    SendMessage $0 ${EM_SETREADONLY} 1 0
  ${Else}
    ${NSD_CreateLabel} 0 32u 100% 12u "(run: hitstor-agent show-id)"
    Pop $0
  ${EndIf}

  ${NSD_CreateLabel} 0 56u 100% 12u "Next steps:"
  Pop $0

  ${NSD_CreateLabel} 12u 70u 100% 48u "1. Open the HitStor dashboard on the server$\r$\n2. Go to Agents > + Add Agent$\r$\n3. Enter this machine's IP, port $ListenPort, and the token above$\r$\n4. Create backup jobs from the Backups section"
  Pop $0

  nsDialogs::Show
FunctionEnd

; ============================================================================
; Required Section: Agent Binary + Config
; ============================================================================

Section "HitStor Agent" SecMain
  SectionIn RO

  ; --- Upgrade: stop existing service before replacing binary ---
  ${If} $IsUpgrade == "1"
    DetailPrint "Stopping existing service..."
    nsExec::ExecToLog '"$INSTDIR\hitstor-agent.exe" service stop'
    Sleep 2000
  ${EndIf}

  ; Install binary.
  SetOutPath "$INSTDIR"
  File "/oname=hitstor-agent.exe" "${BIN_DIR}/hitstor-agent.exe"
  File "hitstor.ico"

  ; Create config directory.
  CreateDirectory "$ConfigDir"
  SetOutPath "$ConfigDir"
  File "/oname=agent-config.yaml.example" "../config/agent-config.yaml.example"

  ; --- Write agent.env (preserve credentials on upgrade) ---
  ${If} $IsUpgrade == "1"
    ; Check if agent is registered (has server address).
    FileOpen $0 "$ConfigDir\agent.env" r
    FileRead $0 $1
    FileClose $0
    ${WordFind} $1 "HITSTOR_AGENT_SERVER=" "E+1{" $2
    ${If} ${Errors}
      ; Not registered — safe to overwrite.
      FileOpen $0 "$ConfigDir\agent.env" w
      FileWrite $0 "HITSTOR_AGENT_LISTEN_PORT=$ListenPort$\r$\n"
      FileClose $0
      DetailPrint "Agent environment file written (fresh)."
    ${Else}
      ; Registered — preserve file, only update listen port.
      DetailPrint "Agent credentials preserved (upgrade)."
    ${EndIf}
  ${Else}
    ; Fresh install.
    FileOpen $0 "$ConfigDir\agent.env" w
    FileWrite $0 "HITSTOR_AGENT_LISTEN_PORT=$ListenPort$\r$\n"
    FileClose $0
    DetailPrint "Agent environment file written."
  ${EndIf}

  ; Write agent-config.yaml if it doesn't exist (preserve on upgrade).
  IfFileExists "$ConfigDir\agent-config.yaml" +3
    FileOpen $0 "$ConfigDir\agent-config.yaml" w
    FileWrite $0 "# Backup paths are configured from the dashboard.$\r$\nagent_paths: []$\r$\n"
    FileClose $0

  ; Add to PATH.
  Call AddToPath

  ; Start Menu shortcuts.
  CreateDirectory "$SMPROGRAMS\HitStor Agent"
  CreateShortcut "$SMPROGRAMS\HitStor Agent\Uninstall HitStor Agent.lnk" "$INSTDIR\uninstall.exe"

  ; Registry for Add/Remove Programs.
  WriteRegStr HKLM "Software\HitStorAgent" "InstallDir" "$INSTDIR"
  WriteRegStr HKLM "Software\HitStorAgent" "ListenPort" "$ListenPort"
  WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\HitStorAgent" \
    "DisplayName" "HitStor Agent"
  WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\HitStorAgent" \
    "DisplayVersion" "${VERSION}"
  WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\HitStorAgent" \
    "UninstallString" "$\"$INSTDIR\uninstall.exe$\""
  WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\HitStorAgent" \
    "DisplayIcon" "$INSTDIR\hitstor.ico"
  WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\HitStorAgent" \
    "Publisher" "HOOPER IT SERVICES"
  WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\HitStorAgent" \
    "InstallLocation" "$INSTDIR"
  WriteRegDWORD HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\HitStorAgent" \
    "NoModify" 1
  WriteRegDWORD HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\HitStorAgent" \
    "NoRepair" 1

  ; Create uninstaller.
  WriteUninstaller "$INSTDIR\uninstall.exe"
SectionEnd

; ============================================================================
; Section: Install as Windows Service (default ON)
; ============================================================================

Section "Install as Windows Service" SecService
  DetailPrint "Installing Windows Service..."

  ${If} $IsUpgrade == "1"
    ; Uninstall old service definition before re-installing (picks up new binary path).
    nsExec::ExecToLog '"$INSTDIR\hitstor-agent.exe" service uninstall'
    Sleep 1000
  ${EndIf}

  nsExec::ExecToLog '"$INSTDIR\hitstor-agent.exe" service install --config "$ConfigDir\agent-config.yaml"'
  Pop $0
  ${If} $0 != 0
    DetailPrint "Warning: service install returned exit code $0"
  ${EndIf}

  ; Add firewall rule.
  DetailPrint "Adding Windows Firewall rule for port $ListenPort..."
  nsExec::ExecToLog 'netsh advfirewall firewall delete rule name="HitStor Agent"'
  nsExec::ExecToLog 'netsh advfirewall firewall add rule name="HitStor Agent" dir=in action=allow protocol=TCP localport=$ListenPort program="$INSTDIR\hitstor-agent.exe"'

  ; Start the service.
  DetailPrint "Starting HitStor Agent service..."
  nsExec::ExecToLog '"$INSTDIR\hitstor-agent.exe" service start'
  Sleep 3000
SectionEnd

; --- Section descriptions ---
!insertmacro MUI_FUNCTION_DESCRIPTION_BEGIN
  !insertmacro MUI_DESCRIPTION_TEXT ${SecMain} "Install the HitStor backup agent binary and configuration."
  !insertmacro MUI_DESCRIPTION_TEXT ${SecService} "Register and start HitStor Agent as a Windows Service (recommended for unattended backups)."
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

  IfFileExists "$ConfigDir\*.*" 0 +3
    ${GetSize} "$ConfigDir" "/S=0K" $R1 $R2 $R3
    IntOp $R0 $R0 + $R1

  WriteRegDWORD HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\HitStorAgent" \
    "EstimatedSize" $R0

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
  nsExec::ExecToLog '"$INSTDIR\hitstor-agent.exe" service stop'
  Sleep 2000
  nsExec::ExecToLog '"$INSTDIR\hitstor-agent.exe" service uninstall'

  ; Remove firewall rule.
  nsExec::ExecToLog 'netsh advfirewall firewall delete rule name="HitStor Agent"'

  ; Remove binary and uninstaller.
  Delete "$INSTDIR\hitstor-agent.exe"
  Delete "$INSTDIR\hitstor.ico"
  Delete "$INSTDIR\uninstall.exe"
  RMDir "$INSTDIR"

  ; Remove config files.
  Delete "$ConfigDir\agent.env"
  Delete "$ConfigDir\agent-config.yaml"
  Delete "$ConfigDir\agent-config.yaml.example"
  Delete "$ConfigDir\agent-token"
  Delete "$ConfigDir\server-ca.crt"
  RMDir "$ConfigDir"

  ; Remove from PATH.
  Call un.RemoveFromPath

  ; Remove Start Menu shortcuts.
  Delete "$SMPROGRAMS\HitStor Agent\Uninstall HitStor Agent.lnk"
  RMDir "$SMPROGRAMS\HitStor Agent"

  ; Remove registry entries.
  DeleteRegKey HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\HitStorAgent"
  DeleteRegKey HKLM "Software\HitStorAgent"
SectionEnd
