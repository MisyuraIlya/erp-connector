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
SetupIconFile={#BuildDir}\{#AppExe}
Compression=lzma
SolidCompression=yes
ArchitecturesAllowed=x64
ArchitecturesInstallIn64BitMode=x64
PrivilegesRequired=admin
UninstallDisplayIcon={app}\{#AppExe}

[Files]
Source: "{#BuildDir}\{#AppExe}"; DestDir: "{app}"; Flags: ignoreversion
Source: "{#BuildDir}\{#ServiceExe}"; DestDir: "{app}"; Flags: ignoreversion

[Icons]
Name: "{autodesktop}\{#AppName}"; Filename: "{app}\{#AppExe}"

[Run]
Filename: "sc.exe"; Parameters: "create {#ServiceName} binPath= ""{app}\{#ServiceExe}"" start= auto"; Flags: runhidden ignoreerrors
Filename: "sc.exe"; Parameters: "description {#ServiceName} ""ERP Connector API Service"""; Flags: runhidden ignoreerrors
Filename: "sc.exe"; Parameters: "start {#ServiceName}"; Flags: runhidden ignoreerrors

[UninstallRun]
Filename: "sc.exe"; Parameters: "stop {#ServiceName}"; Flags: runhidden ignoreerrors
Filename: "sc.exe"; Parameters: "delete {#ServiceName}"; Flags: runhidden ignoreerrors
