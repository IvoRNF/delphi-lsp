let SessionLoad = 1
let s:so_save = &g:so | let s:siso_save = &g:siso | setg so=0 siso=0 | setl so=-1 siso=-1
let v:this_session=expand("<sfile>:p")
doautoall SessionLoadPre
silent only
silent tabonly
cd C:/delphi-lsp
if expand('%') == '' && !&modified && line('$') <= 1 && getline(1) == ''
  let s:wipebuf = bufnr('%')
endif
let s:shortmess_save = &shortmess
set shortmess+=aoO
badd +2 internal/lsp/completion_debug_test.go
badd +85 internal/lsp/config.go
badd +1 term://C:/delphi-lsp//3364:go\ doc\ -C\ \"internal/lsp\"\ \"resolve\"
badd +0 C:/delphi-lsp
badd +270 internal/lsp/document.go
badd +15 README.md
badd +0 term://C:/delphi-lsp//13868:C:/Windows/System32/WindowsPowerShell/v1.0/powershell.exe
argglobal
%argdel
$argadd C:/delphi-lsp
argglobal
if bufexists(fnamemodify("term://C:/delphi-lsp//13868:C:/Windows/System32/WindowsPowerShell/v1.0/powershell.exe", ":p")) | buffer term://C:/delphi-lsp//13868:C:/Windows/System32/WindowsPowerShell/v1.0/powershell.exe | else | edit term://C:/delphi-lsp//13868:C:/Windows/System32/WindowsPowerShell/v1.0/powershell.exe | endif
if &buftype ==# 'terminal'
  silent file term://C:/delphi-lsp//13868:C:/Windows/System32/WindowsPowerShell/v1.0/powershell.exe
endif
balt README.md
setlocal foldmethod=manual
setlocal foldexpr=0
setlocal foldmarker={{{,}}}
setlocal foldignore=#
setlocal foldlevel=0
setlocal foldminlines=1
setlocal foldnestmax=20
setlocal foldenable
let s:l = 5 - ((4 * winheight(0) + 24) / 48)
if s:l < 1 | let s:l = 1 | endif
keepjumps exe s:l
normal! zt
keepjumps 5
normal! 018|
lcd C:/delphi-lsp
tabnext 1
if exists('s:wipebuf') && len(win_findbuf(s:wipebuf)) == 0 && getbufvar(s:wipebuf, '&buftype') isnot# 'terminal'
  silent exe 'bwipe ' . s:wipebuf
endif
unlet! s:wipebuf
set winheight=1 winwidth=20
let &shortmess = s:shortmess_save
let s:sx = expand("<sfile>:p:r")."x.vim"
if filereadable(s:sx)
  exe "source " . fnameescape(s:sx)
endif
let &g:so = s:so_save | let &g:siso = s:siso_save
set hlsearch
doautoall SessionLoadPost
unlet SessionLoad
" vim: set ft=vim :
