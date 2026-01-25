#define AppName "ERP Connector"
#define AppPublisher "Digitrage"
#define AppExe "erp-connector.exe"
#define ServiceExe "erp-connectord.exe"
#define ServiceName "erp-connectord"

#ifndef AppVersion
#define AppVersion "0.0.0"
#endif

#ifndef BuildDir
#define BuildDir "."
#endif

#ifndef OutputDir
#define OutputDir "."
#endif

[Setup]
AppId={{715C208D-CBF9-40CA-B1D2-E1E4C3BBEC5E}
AppName={#AppName}
AppVersion={#AppVersion}
AppPublisher={#AppPublisher}
DefaultDirName={pf}\erp-connector
DefaultGroupName={#AppName}
DisableProgramGroupPage=yes
OutputDir={#OutputDir}
OutputBaseFilename=erp-connector-setup-{#AppVersion}
Compression=lzma
SolidCompression=yes
ArchitecturesAllowed=x64
ArchitecturesInstallIn64BitMode=x64
PrivilegesRequired=admin
UninstallDisplayIcon={app}\{#AppExe}
SetupIconFile={#SourcePath}\icon.ico

[Files]
Source: "{#BuildDir}\{#AppExe}"; DestDir: "{app}"; Flags: ignoreversion
Source: "{#BuildDir}\{#ServiceExe}"; DestDir: "{app}"; Flags: ignoreversion

[Icons]
Name: "{autodesktop}\{#AppName}"; Filename: "{sys}\WindowsPowerShell\v1.0\powershell.exe"; Parameters: "-NoProfile -WindowStyle Hidden -Command ""Start-Process -FilePath '{app}\{#AppExe}' -WorkingDirectory '{app}' -Verb RunAs"""; WorkingDir: "{app}"; IconFilename: "{app}\{#AppExe}"

[Run]
Filename: "{cmd}"; Parameters: "/C sc.exe create {#ServiceName} binPath= ""{app}\{#ServiceExe}"" start= auto >nul 2>&1 & exit /b 0"; Flags: runhidden
Filename: "{cmd}"; Parameters: "/C sc.exe description {#ServiceName} ""ERP Connector API Service"" >nul 2>&1 & exit /b 0"; Flags: runhidden
Filename: "{cmd}"; Parameters: "/C sc.exe start {#ServiceName} >nul 2>&1 & exit /b 0"; Flags: runhidden

[UninstallRun]
Filename: "{cmd}"; Parameters: "/C sc.exe stop {#ServiceName} >nul 2>&1 & exit /b 0"; Flags: runhidden
Filename: "{cmd}"; Parameters: "/C sc.exe delete {#ServiceName} >nul 2>&1 & exit /b 0"; Flags: runhidden
