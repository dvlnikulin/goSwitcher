; Скрипт инсталлятора для GoSwitcher
#define MyAppName "GoSwitcher"
#define MyAppVersion "1.4.11"
#define MyAppPublisher "NikoDim"
#define MyAppExeName "GoSwitcher.exe"

[Setup]
; Уникальный идентификатор приложения (не менять при будущих обновлениях)
AppId={{8F4B2C9A-3E1D-4A7B-9C5E-1D2F3A4B5C6D}
AppName={#MyAppName}
AppVersion={#MyAppVersion}
AppPublisher={#MyAppPublisher}
; Установка в профиль пользователя без требования прав администратора (UAC)
DefaultDirName={localappdata}\Programs\{#MyAppName}
DefaultGroupName={#MyAppName}
PrivilegesRequired=lowest
DisableProgramGroupPage=yes
; Куда положить готовый установщик и как его назвать
OutputDir=dist
OutputBaseFilename=GoSwitcher_Setup_v{#MyAppVersion}
SetupIconFile=icon.ico
Compression=lzma2/ultra64
SolidCompression=yes
WizardStyle=modern
; Автоматическое закрытие работающего экземпляра при обновлении
CloseApplications=force
RestartApplications=no

[Languages]
Name: "russian"; MessagesFile: "compiler:Languages\Russian.isl"
Name: "english"; MessagesFile: "compiler:Default.isl"

[Tasks]
Name: "desktopicon"; Description: "{cm:CreateDesktopIcon}"; GroupDescription: "{cm:AdditionalIcons}"; Flags: unchecked
Name: "autostart"; Description: "Запускать GoSwitcher автоматически при входе в Windows"; GroupDescription: "Дополнительно:"; Flags: checkedonce

[Files]
; Основной исполняемый файл (HTML и иконка уже вшиты внутрь через go:embed)
Source: "{#MyAppExeName}"; DestDir: "{app}"; Flags: ignoreversion
; Предобученная модель Наивного Байеса
Source: "model.json"; DestDir: "{app}"; Flags: ignoreversion
; Словари (нужны для работы кнопки «Переобучить ИИ» из Web UI)
Source: "words_ru.txt"; DestDir: "{app}"; Flags: ignoreversion skipifsourcedoesntexist
Source: "words_en.txt"; DestDir: "{app}"; Flags: ignoreversion skipifsourcedoesntexist

[Icons]
Name: "{group}\{#MyAppName}"; Filename: "{app}\{#MyAppExeName}"
Name: "{autodesktop}\{#MyAppName}"; Filename: "{app}\{#MyAppExeName}"; Tasks: desktopicon

[Registry]
; Прописываем автозапуск в реестре текущего пользователя, если выбрана галочка
Root: HKCU; Subkey: "Software\Microsoft\Windows\CurrentVersion\Run"; ValueType: string; ValueName: "{#MyAppName}"; ValueData: """{app}\{#MyAppExeName}"""; Flags: uninsdeletevalue; Tasks: autostart

[Run]
; Запуск программы сразу после завершения установки
Filename: "{app}\{#MyAppExeName}"; Description: "{cm:LaunchProgram,{#StringChange(MyAppName, '&', '&&')}}"; Flags: nowait postinstall skipifsilent

[UninstallDelete]
; При полном удалении подчищаем сгенерированные файлы конфигурации и статистики
Type: files; Name: "{app}\settings.json"
Type: files; Name: "{app}\stats.json"
Type: files; Name: "{app}\heatmap.json"
Type: dirifempty; Name: "{app}"

[Code]
// Принудительно убиваем фоновый процесс GoSwitcher.exe перед установкой/обновлением или удалением
procedure KillRunningProcess();
var
  ResultCode: Integer;
begin
  Exec('taskkill.exe', '/F /IM {#MyAppExeName} /T', '', SW_HIDE, ewWaitUntilTerminated, ResultCode);
  Sleep(300);
end;

function PrepareToInstall(var NeedsRestart: Boolean): String;
begin
  KillRunningProcess();
  Result := '';
end;

procedure CurUninstallStepChanged(CurUninstallStep: TUninstallStep);
begin
  if CurUninstallStep = usUninstall then
  begin
    KillRunningProcess();
  end;
end;
