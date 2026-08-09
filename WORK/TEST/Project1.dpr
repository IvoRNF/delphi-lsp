program Project1;

{$APPTYPE CONSOLE}

uses
  SysUtils,Classes;


var
  LList: TStringList;


begin
  try
    { TODO -oUser -cConsole Main : Insert code here }

    Write('foo');
    Readln;
  except
    on E:Exception do
      Writeln(E.Classname, ': ', E.Message);
  end;
end.
