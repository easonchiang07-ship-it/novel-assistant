; Inno Setup script for 小說助手 (Novel Assistant)
; Usage:
;   ISCC /DMyAppVersion=v1.0.0 scripts\windows.iss

#ifndef MyAppVersion
  #define MyAppVersion "dev"
#endif

#define MyAppName      "小說助手"
#define MyAppPublisher "novel-assistant"
#define MyAppExeName   "novel-assistant.exe"
#define MyAppURL       "https://github.com/easonchiang07-ship-it/novel-assistant"

[Setup]
AppId={{A3B5C7D9-E1F2-4A3B-8C5D-6E7F8A9B0C1D}
AppName={#MyAppName}
AppVersion={#MyAppVersion}
AppPublisher={#MyAppPublisher}
AppPublisherURL={#MyAppURL}
AppSupportURL={#MyAppURL}/issues
DefaultDirName={autopf}\NovelAssistant
DefaultGroupName={#MyAppName}
DisableProgramGroupPage=yes
OutputDir=Output
OutputBaseFilename=NovelAssistantSetup
SetupIconFile=
Compression=lzma2/ultra64
SolidCompression=yes
WizardStyle=modern
; Require Windows 10 or later
MinVersion=10.0
; Don't require admin — install to user's AppData if not admin
PrivilegesRequiredOverridesAllowed=commandline dialog

[Languages]
Name: "chinesesimplified"; MessagesFile: "compiler:Languages\ChineseSimplified.isl"
Name: "english"; MessagesFile: "compiler:Default.isl"

[Tasks]
Name: "desktopicon"; Description: "建立桌面捷徑"; GroupDescription: "其他工作:"; Flags: unchecked
Name: "startuprun";  Description: "登入時自動啟動"; GroupDescription: "其他工作:"; Flags: unchecked

[Files]
Source: "{#MyAppExeName}"; DestDir: "{app}"; Flags: ignoreversion

[Icons]
Name: "{group}\{#MyAppName}";          Filename: "{app}\{#MyAppExeName}"
Name: "{group}\解除安裝 {#MyAppName}"; Filename: "{uninstallexe}"
Name: "{autodesktop}\{#MyAppName}";    Filename: "{app}\{#MyAppExeName}"; Tasks: desktopicon

[Registry]
Root: HKCU; Subkey: "Software\Microsoft\Windows\CurrentVersion\Run"; \
  ValueType: string; ValueName: "{#MyAppName}"; \
  ValueData: """{app}\{#MyAppExeName}"""; \
  Flags: uninsdeletevalue; Tasks: startuprun

[Run]
; Launch the app after installation (opens browser automatically)
Filename: "{app}\{#MyAppExeName}"; \
  Description: "立即啟動 {#MyAppName}"; \
  Flags: nowait postinstall skipifsilent shellexec

[UninstallRun]
Filename: "taskkill"; Parameters: "/f /im {#MyAppExeName}"; Flags: runhidden

[Code]
// Check if port 8080 is likely free (basic heuristic — skip if needed)
function InitializeSetup(): Boolean;
begin
  Result := True;
end;
