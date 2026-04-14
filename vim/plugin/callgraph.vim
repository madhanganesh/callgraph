if exists('g:loaded_callgraph')
  finish
endif
let g:loaded_callgraph = 1

command! CallGraph           call callgraph#show('callers')
command! CallGraphCallees    call callgraph#show('callees')
command! CallGraphSummarize  call callgraph#summarize()
command! CallGraphYankFile        call callgraph#yank_file()
command! CallGraphYankFileLine    call callgraph#yank_file_line()
command! CallGraphYankFileMethod  call callgraph#yank_file_method()
command! CallGraphPrompt          call callgraph#prompt()

nnoremap <silent> <Plug>(callgraph)              :call callgraph#show('callers')<CR>
nnoremap <silent> <Plug>(callgraph-callees)      :call callgraph#show('callees')<CR>
nnoremap <silent> <Plug>(callgraph-summarize)    :call callgraph#summarize()<CR>
nnoremap <silent> <Plug>(callgraph-yank-file)    :call callgraph#yank_file()<CR>
nnoremap <silent> <Plug>(callgraph-yank-line)    :call callgraph#yank_file_line()<CR>
nnoremap <silent> <Plug>(callgraph-yank-method)  :call callgraph#yank_file_method()<CR>
nnoremap <silent> <Plug>(callgraph-prompt)       :call callgraph#prompt()<CR>

if !hasmapto('<Plug>(callgraph)') && maparg('<leader>cc', 'n') ==# ''
  nmap <leader>cc <Plug>(callgraph)
endif

if !hasmapto('<Plug>(callgraph-callees)') && maparg('<leader>cd', 'n') ==# ''
  nmap <leader>cd <Plug>(callgraph-callees)
endif

if !hasmapto('<Plug>(callgraph-summarize)') && maparg('<leader>cs', 'n') ==# ''
  nmap <leader>cs <Plug>(callgraph-summarize)
endif

if !hasmapto('<Plug>(callgraph-yank-file)') && maparg('<leader>cf', 'n') ==# ''
  nmap <leader>cf <Plug>(callgraph-yank-file)
endif

if !hasmapto('<Plug>(callgraph-yank-line)') && maparg('<leader>cl', 'n') ==# ''
  nmap <leader>cl <Plug>(callgraph-yank-line)
endif

if !hasmapto('<Plug>(callgraph-yank-method)') && maparg('<leader>cm', 'n') ==# ''
  nmap <leader>cm <Plug>(callgraph-yank-method)
endif

if !hasmapto('<Plug>(callgraph-prompt)') && maparg('<leader>cp', 'n') ==# ''
  nmap <leader>cp <Plug>(callgraph-prompt)
endif
