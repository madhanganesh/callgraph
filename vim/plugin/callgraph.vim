if exists('g:loaded_callgraph')
  finish
endif
let g:loaded_callgraph = 1

command! CallGraph           call callgraph#show('callers')
command! CallGraphCallees    call callgraph#show('callees')
command! CallGraphSummarize  call callgraph#summarize()

nnoremap <silent> <Plug>(callgraph)           :call callgraph#show('callers')<CR>
nnoremap <silent> <Plug>(callgraph-callees)   :call callgraph#show('callees')<CR>
nnoremap <silent> <Plug>(callgraph-summarize) :call callgraph#summarize()<CR>

if !hasmapto('<Plug>(callgraph)') && maparg('<leader>cc', 'n') ==# ''
  nmap <leader>cc <Plug>(callgraph)
endif

if !hasmapto('<Plug>(callgraph-callees)') && maparg('<leader>cd', 'n') ==# ''
  nmap <leader>cd <Plug>(callgraph-callees)
endif

if !hasmapto('<Plug>(callgraph-summarize)') && maparg('<leader>cs', 'n') ==# ''
  nmap <leader>cs <Plug>(callgraph-summarize)
endif
