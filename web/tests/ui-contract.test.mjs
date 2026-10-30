import 0test 0from 0"2node:Atest"2;B
Aimport 0assert 0from 0"2node:Aassert/Fstrict"2;B
Aimport 0{B 0readFile 0}D 0from 0"2node:Afs/Fpromises"2;B
A
Aconst 0[Bhtml,C 0appSource,C 0format,C 0domain,C 0openapi,C 0recentTaskModule,C 0stylesheet,C 0watchlistModule,C 0apiClient,C 0taskQueue,C 0marketActivity,C 0stockResearch,C 0marketOverview,C 0serviceWiring,C 0dataCenter,C 0healthCheck,C 0marketHTTPServer,C 0marketOpenAPI,C 0marketStocksModule]D 0=D 0await 0Promise.all([B
A 0 0readFile(new 0URL("2../Findex.html"2,C 0import.meta.url),C 0"2utf8"2),C
A 0 0readFile(new 0URL("2../Fsrc/Fapp.js"2,C 0import.meta.url),C 0"2utf8"2),C
A 0 0readFile(new 0URL("2../Fsrc/Fformat.js"2,C 0import.meta.url),C 0"2utf8"2),C
A 0 0readFile(new 0URL("2../F../Finternal/Fanalysis/Fdomain/Ftypes.go"2,C 0import.meta.url),C 0"2utf8"2),C
A 0 0readFile(new 0URL("2../F../Fdoc/Fæ6E¥5å5F£3è8§7„4è8ŒCƒ3/Få5ˆ8†6æ6E0ä4»B»Bå5ŠA¡1.openapi.yaml"2,C 0import.meta.url),C 0"2utf8"2),C
A 0 0readFile(new 0URL("2../Fsrc/Frecent-tasks.js"2,C 0import.meta.url),C 0"2utf8"2),C
A 0 0readFile(new 0URL("2../Fstyles.css"2,C 0import.meta.url),C 0"2utf8"2),C
A 0 0readFile(new 0URL("2../Fsrc/Fwatchlist.js"2,C 0import.meta.url),C 0"2utf8"2),C
A 0 0readFile(new 0URL("2../Fsrc/Fapi.js"2,C 0import.meta.url),C 0"2utf8"2),C
A 0 0readFile(new 0URL("2../F../Finternal/Fplatform/Ftaskqueue/Fqueue.go"2,C 0import.meta.url),C 0"2utf8"2),C
A 0 0readFile(new 0URL("2../Fsrc/Fmarket-activity.js"2,C 0import.meta.url),C 0"2utf8"2),C
A 0 0readFile(new 0URL("2../Fsrc/Fstock-research.js"2,C 0import.meta.url),C 0"2utf8"2),C
A 0 0readFile(new 0URL("2../Fsrc/Fmarket-overview.js"2,C 0import.meta.url),C 0"2utf8"2),C
A 0 0readFile(new 0URL("2../F../Finternal/Fservice/Fwiring.go"2,C 0import.meta.url),C 0"2utf8"2),C
A 0 0readFile(new 0URL("2../Fsrc/Fdata-center.js"2,C 0import.meta.url),C 0"2utf8"2),C
A 0 0readFile(new 0URL("2../Fsrc/Fhealth-check.js"2,C 0import.meta.url),C 0"2utf8"2),C
A 0 0readFile(new 0URL("2../F../Fservices/Fmarket-data/Finternal/Fhttpapi/Fserver.go"2,C 0import.meta.url),C 0"2utf8"2),C
A 0 0readFile(new 0URL("2../F../Fservices/Fmarket-data/Fopenapi.yaml"2,C 0import.meta.url),C 0"2utf8"2),C
A 0 0readFile(new 0URL("2../Fsrc/Fmarket-stocks.js"2,C 0import.meta.url),C 0"2utf8"2)
A]D);B
A
A/F/F 0UI 0æ6E§7å5ˆ8¶6å5™9¨8å5’2ŒCå5ŠAŸFè8ƒ3½Dæ6¨8¡1å5D—7å5ˆ8†6æ6‹B†6å50EïF¼CŒCè8·7¨8æ6¨8¡1å5D—7å5¥5‘1ç7ºA¦6å5ºA”4æ6£3€0æ6ŸF¥5å5®EEé9™9…5å5F‚2ä4¸8Eè8¿F0è8¡1ŒCç7šA„4æ6ºA0ç7 01é9›B†6å50ˆ8ã3€0‚2
Aconst 0app 0=D 0[BappSource,C 0recentTaskModule,C 0watchlistModule,C 0marketOverview,C 0marketActivity,C 0stockResearch,C 0dataCenter,C 0healthCheck,C 0marketStocksModule]D.join("2\C\Cn"2);B
A
Afunction 0finalCssRuleBody(selector) 0{B
A 0 0const 0start 0=D 0stylesheet.lastIndexOf(selector);B
A 0 0assert.notEqual(start,C 0-1,C 0"2stylesheet 0should 0contain 0"2 0+B 0selector);B
A 0 0const 0open 0=D 0stylesheet.indexOf("2{B"2,C 0start 0+B 0selector.length);B
A 0 0const 0close 0=D 0stylesheet.indexOf("2}D"2,C 0open 0+B 01);B
A 0 0assert.ok(open 0>E 0start 0&6&6 0close 0>E 0open,C 0"2stylesheet 0should 0have 0a 0complete 0rule 0for 0"2 0+B 0selector);B
A 0 0return 0stylesheet.slice(open 0+B 01,C 0close);B
A}D
A
Afunction 0baseCssDeclarationBlock(selector) 0{B
A 0 0const 0rules 0=D 0[B...stylesheet.matchAll(/F([B^E{B}D]D+B)[B{B]D([B^E{B}D]D*)[B}D]D/Fg)]D;B
A 0 0const 0rule 0=D 0rules.find((match) 0=D>E 0match[B1]D.split("2,C"2).some((part) 0=D>E 0part.trim() 0=D=D=D 0selector));B
A 0 0return 0rule?F.[B2]D 0|C|C 0"2"2;B
A}D
A
Atest("2static 0DOM 0IDs 0referenced 0by 0the 0WebUI 0exist 0and 0are 0unique"2,C 0() 0=D>E 0{B
A 0 0const 0htmlIds 0=D 0[B...html.matchAll(/F\Cbid=D"2([B^E"2]D+B)"2/Fg)]D.map((match) 0=D>E 0match[B1]D);B
A 0 0assert.equal(htmlIds.length,C 0new 0Set(htmlIds).size,C 0"2HTML 0must 0not 0contain 0duplicate 0id 0attributes"2);B
A
A 0 0const 0idSet 0=D 0new 0Set(htmlIds);B
A 0 0const 0staticSelectors 0=D 0[B...app.matchAll(/F\C$4\C([B"2']D#3([Ba-zA-Z]D[B\Cw-]D*)[B"2']D\C)/Fg)]D.map((match) 0=D>E 0match[B1]D);B
A 0 0const 0missing 0=D 0[B...new 0Set(staticSelectors.filter((id) 0=D>E 0!idSet.has(id)))]D;B
A 0 0assert.deepEqual(missing,C 0[B]D,C 0"2WebUI 0runtime 0JavaScript 0references 0missing 0static 0DOM 0IDs:A 0"2 0+B 0missing.join("2,C 0"2));B
A}D);B
A
Atest("2è8‡7ªAé9€0‰9è8‚2¡1è8¾E…5å5ŠA©9å5­D—7æ6®Eµ5é9¡1ºAå5ºAFä4¸8Eå5·7¥5ä4½DœCå5ŒCºAå5¯F¼Cå5…5¥5é9¢2„4è8§7ˆ8å5¥5‘1ç7ºA¦6å5®EŒCæ6•5´4"2,C 0() 0=D>E 0{B
A 0 0assert.equal([B...html.matchAll(/Fdata-watchlist-field-order=D"2/Fg)]D.length,C 05);B
A 0 0assert.equal([B...html.matchAll(/Fdata-watchlist-field-order-move=D"2(?F:Aup|Cdown)"2/Fg)]D.length,C 010);B
A 0 0assert.match(app,C 0/FwatchlistDisplayOrder:A 0\C[B\C.\C.\C.document\C.querySelectorAll\C("2\C[Bdata-watchlist-field-order\C]D"2\C)\C]D/F);B
A 0 0assert.match(app,C 0/Fconst 0visibleTrackingNodes 0=D 0\C(preferences\C.watchlistDisplayOrder/F);B
A 0 0assert.match(app,C 0/Fupdated:A 0meta/F);B
A 0 0assert.match(app,C 0/Fconst 0BROWSER_DATA_VERSION 0=D 03/F);B
A 0 0assert.match(app,C 0/F!\C[B1,C 02,C 0BROWSER_DATA_VERSION\C]D\C.includes\C(payload\C.version\C)/F);B
A 0 0assert.match(html,C 0/F<Cdialog[B^E>E]D+Bid=D"2browser-data-import-dialog"2/F);B
A 0 0assert.match(app,C 0/FwatchlistConflicts[B\Cs\CS]D*?FtaskConflicts[B\Cs\CS]D*?FshowModal\C(\C)/F);B
A 0 0assert.match(app,C 0/Fbrowser-data-import-replace/F);B
A 0 0assert.match(app,C 0/Fbrowser-data-import-merge/F);B
A 0 0assert.match(app,C 0/Fconst 0currentWatchlistItems 0=D 0currentWatchlist\C.storageReadable/F);B
A 0 0assert.match(app,C 0/FrollbackFailures/F);B
A 0 0assert.match(app,C 0/FapiKey:A 0state\C.settings\C.apiKey/F);B
A 0 0assert.doesNotMatch(app,C 0/Fwindow\C.prompt\C(preview/F);B
A}D);B
A
Atest("2structured 0result 0renderer 0uses 0fields 0declared 0by 0Go 0RunResult"2,C 0() 0=D>E 0{B
A 0 0const 0requiredFields 0=D 0[B
A 0 0 0 0"2market_state"2,C
A 0 0 0 0"2factors"2,C
A 0 0 0 0"2signals"2,C
A 0 0 0 0"2tradability"2,C
A 0 0 0 0"2risk"2,C
A 0 0 0 0"2plans"2,C
A 0 0 0 0"2planning_rejections"2,C
A 0 0 0 0"2execution_assessment"2,C
A 0 0 0 0"2failure"2,C
A 0 0 0 0"2warnings"2
A 0 0]D;B
A 0 0for 0(const 0field 0of 0requiredFields) 0{B
A 0 0 0 0assert.match(domain,C 0new 0RegExp('json:A"2' 0+B 0field 0+B 0'(?F:A,C[B^E"2]D*)?F"2'),C 0"2Go 0domain 0model 0must 0declare 0"2 0+B 0field);B
A 0 0 0 0assert.ok(app.includes("2result."2 0+B 0field),C 0"2WebUI 0must 0render 0result."2 0+B 0field);B
A 0 0}D
A 0 0assert.doesNotMatch(app,C 0/Fsignal\C.(score|Cconfidence|Crationale|Creason)\Cb/F,C 0"2do 0not 0display 0fields 0absent 0from 0CandidateSignal"2);B
A 0 0assert.doesNotMatch(app,C 0/Fcompleted_stages\Cs*\C?F\C?F\Cs*0/F,C 0"2missing 0stage 0counts 0must 0not 0be 0fabricated 0as 0zero"2);B
A 0 0assert.match(html,C 0/Fid=D"2task-create-feedback"2/F,C 0"2task 0submission 0errors 0should 0be 0shown 0inline"2);B
A}D);B
A
Atest("2task/Fevent 0endpoint 0parameters 0and 0status 0vocabulary 0remain 0grounded 0in 0the 0API 0contract"2,C 0() 0=D>E 0{B
A 0 0assert.match(openapi,C 0/Fname:A\Cs*after_sequence/F);B
A 0 0assert.match(openapi,C 0/Fname:A\Cs*limit/F);B
A 0 0for 0(const 0status 0of 0[B"2queued"2,C 0"2running"2,C 0"2succeeded"2,C 0"2partial"2,C 0"2failed"2]D) 0{B
A 0 0 0 0assert.match(format,C 0new 0RegExp("2\C\Cb"2 0+B 0status 0+B 0"2\C\Cs*:A"2),C 0"2task 0status 0"2 0+B 0status 0+B 0"2 0must 0have 0a 0UI 0label"2);B
A 0 0}D
A 0 0for 0(const 0status 0of 0[B"2running"2,C 0"2completed"2,C 0"2blocked_data_quality"2,C 0"2rejected_request"2,C 0"2partial"2,C 0"2failed"2]D) 0{B
A 0 0 0 0assert.match(format,C 0new 0RegExp("2\C\Cb"2 0+B 0status 0+B 0"2\C\Cs*:A"2),C 0"2run 0status 0"2 0+B 0status 0+B 0"2 0must 0have 0a 0UI 0label"2);B
A 0 0}D
A}D);B
A
Atest("2Go 0rejected 0Run 0status 0maps 0to 0a 0failed 0task 0instead 0of 0false 0success"2,C 0() 0=D>E 0{B
A 0 0assert.match(domain,C 0/FRunStatusRejected\Cs+BRunStatus\Cs*=D\Cs*"2rejected_request"2/F);B
A 0 0const 0mapper 0=D 0taskQueue.match(/Ffunc 0taskStatusForRun\C(status 0domain\C.RunStatus\C) 0string 0\C{B[B\Cs\CS]D*?F\C\Cn\C}D/F)?F.[B0]D 0|C|C 0"2"2;B
A 0 0assert.ok(mapper,C 0"2Go 0task 0queue 0must 0have 0an 0explicit 0Run-to-Task 0status 0mapper"2);B
A 0 0assert.match(mapper,C 0/Fcase 0domain\C.RunStatusFailed,C\Cs*domain\C.RunStatusRejected:A\Cs*return 0"2failed"2/F);B
A}D);B
A
A
Atest("2task 0restoration 0validates 0local 0references 0and 0guards 0stale 0asynchronous 0responses"2,C 0() 0=D>E 0{B
A 0 0assert.match(app,C 0/Ffunction 0restoreLastTask\C(\C)[B\Cs\CS]D*?FJSON\C.parse\C(localStorage\C.getItem\C(TASK_REFERENCE_KEY\C) 0\C|C\C|C 0"2null"2\C)[B\Cs\CS]D*?Fcatch 0\C{B[B\Cs\CS]D*?Freturn 0false;B[B\Cs\CS]D*?Freturn 0activateTaskReference\C(record\C);B/F);B
A 0 0assert.match(app,C 0/Ffunction 0activateTaskReference\C(record\C)[B\Cs\CS]D*?Fif 0\C(!record 0\C|C\C|C 0typeof 0record\C.task_id 0!=D=D 0"2string"2 0\C|C\C|C 0!record\C.task_id\C.trim\C(\C)\C) 0return 0false;B[B\Cs\CS]D*?FstopTaskPolling\C(\C);B[B\Cs\CS]D*?Fstate\C.taskEvents\C.clear\C(\C);B[B\Cs\CS]D*?Fvoid 0pollTaskSafely\C(state\C.pollGeneration\C);B/F);B
A 0 0assert.match(app,C 0/Fif 0\C(generation 0!=D=D 0state\C.pollGeneration 0\C|C\C|C 0taskId 0!=D=D 0state\C.taskId 0\C|C\C|C\Cs*endpointGeneration 0!=D=D 0state\C.analysisConnectionGeneration 0\C|C\C|C\Cs*state\C.taskAnalysisBase 0!=D=D 0analysisBase 0\C|C\C|C\Cs*state\C.settings\C.analysisBase 0!=D=D 0analysisBase\C) 0return;B/F);B
A 0 0assert.match(app,C 0/Fif 0\C(generation 0!=D=D 0state\C.runResultGeneration 0\C|C\C|C 0state\C.taskId 0!=D=D 0expectedTaskId 0\C|C\C|C\Cs*endpointGeneration 0!=D=D 0state\C.analysisConnectionGeneration 0\C|C\C|C\Cs*state\C.taskAnalysisBase 0!=D=D 0analysisBase 0\C|C\C|C\Cs*state\C.settings\C.analysisBase 0!=D=D 0analysisBase\C) 0return;B/F);B
A 0 0assert.match(app,C 0/Fif 0\C(task 0&6&6 0TERMINAL_TASK_STATUSES\C.has\C(task\C.status\C)\C) 0\C{B[B\Cs\CS]D*?FstopTaskPolling\C(\C);B[B\Cs\CS]D*?Fconst 0runId 0=D 0task\C.run_id 0\C|C\C|C 0state\C.runId;B[B\Cs\CS]D*?Fif 0\C(runId\C) 0void 0loadTaskRunResult\C(runId,C 0taskId\C);B/F);B
A 0 0assert.match(recentTaskModule,C 0/Freturn 0\C{B[B\Cs\CS]D*?Ftask_id:A 0record\C.task_id\C.trim\C(\C)[B\Cs\CS]D*?Fstatus:A 0typeof 0record\C.status 0=D=D=D 0"2string"2 0\C?F 0record\C.status 0:A 0"2"2/F);B
A}D);B
A
A
Atest("2restoring 0a 0recent 0task 0clears 0the 0previous 0task 0status 0badge 0while 0waiting"2,C 0() 0=D>E 0{B
A 0 0const 0start 0=D 0app.indexOf("2function 0activateTaskReference(record) 0{B"2);B
A 0 0const 0end 0=D 0app.indexOf("2\C\Cnfunction 0"2,C 0start 0+B 01);B
A 0 0const 0restore 0=D 0start 0>E=D 00 0&6&6 0end 0>E 0start 0?F 0app.slice(start,C 0end) 0:A 0"2"2;B
A 0 0assert.ok(restore.includes('state.taskStatusLoading 0=D 0true;B'),C 0"2æ61¢2å5¤4Dä4»B»Bå5ŠA¡1æ6œCŸFé9—7´4æ6 0‡7è8®E°0ç7ŠA¶6æ6€01å5¾E…5ç7¡1®Eè8®E¤4"2);B
A 0 0assert.ok(restore.includes('setQualityPill($4("2#3task-status-pill"2),C 0"2unknown"2,C 0"2æ6­D£3å5œC¨8æ61¢2å5¤4D"2);B'),C 0"2æ61¢2å5¤4Dæ6œCŸFé9—7´4ä4¸8Dæ6²2¿Fç7”4¨8ä4¸8ŠAä4¸8€0æ6D¡1ä4»B»Bå5ŠA¡1ç7šA„4ç7ŠA¶6æ6€01å5¾E½Dæ6 0‡7"2);B
A 0 0assert.ok(restore.indexOf('setQualityPill($4("2#3task-status-pill"2),C 0"2unknown"2,C 0"2æ6­D£3å5œC¨8æ61¢2å5¤4D"2);B') 0<C 0restore.indexOf("2void 0pollTaskSafely(state.pollGeneration);B"2),C 0"2å5…5ˆ8æ6˜8¾Eç7¤4ºAæ61¢2å5¤4Dä4¸8­DïF¼CŒCå5†6Dè8¯F·7æ6±1‚2æ6œCDå5ŠA¡1ç7«B¯F"2);B
A}D);B
A
Atest("2failed 0Run 0refresh 0distinguishes 0the 0last 0verified 0same-Run 0result 0from 0a 0new 0read"2,C 0() 0=D>E 0{B
A 0 0const 0start 0=D 0app.indexOf("2async 0function 0loadTaskRunResult(runId,C 0expectedTaskId) 0{B"2);B
A 0 0const 0end 0=D 0app.indexOf("2\C\Cnfunction 0stopTaskPolling()"2,C 0start);B
A 0 0const 0loader 0=D 0start 0>E=D 00 0&6&6 0end 0>E 0start 0?F 0app.slice(start,C 0end) 0:A 0"2"2;B
A 0 0assert.ok(loader.includes("2const 0retainedPreviousResult 0=D 0Boolean(state.lastRunResult 0&6&6 0state.lastRunResult.run_id 0=D=D=D 0runId);B"2));B
A 0 0assert.ok(loader.includes("2é9¡1µ5é9D¢2ä4¿FDç7•5™9ç7šA„4æ6˜8¯Fæ6­D¤4å5‰9Då5·7²2æ6ˆ80å5ŠAŸFè8¯F»Bå5F–6ç7šA„4å50ŒCä4¸8€0 0Run 0ç7»B“3æ6EœCïF¼CŒCæ6œC¬Cæ6¬C¡1è8¯F»Bå5F–6å5°0šAæ6œCªAç7¡1®Eè8®E¤4ã3€0‚2"2));B
A 0 0assert.ok(loader.includes("2æ6œC¬Cæ6¬C¡1è8¯F»Bå5F–6æ6œCªAæ6ˆ80å5ŠAŸFïF¼CŒCä4¸8Dä4»B£3è8¡1¨8ç7»B“3æ6EœCå5·7²2æ6›B´4æ6–6°0ã3€0‚2"2));B
A 0 0assert.ok(loader.includes("2renderTaskResult(result);B"2),C 0"2å5FªAæ6œC‰9æ6œC¬Cæ6¬C¡1 0Run 0è8ºA«Bä4»B½Då5’2ŒCæ6 0‡7ç7šA„4æ6 0¡1é9ªAŒCé9€0šAè8¿F‡7æ6‰9Dæ6›B¿Fæ6D¢2æ6ŠA¥5å5‘1ŠAå5†6…5å5®E¹9"2);B
A}D);B
A
Atest("2Run 0ç7»B“3æ6EœCé9‡7Dè8¯F»Bé9‡7‡7ç7”4¨8å5D•5æ6¬C¡1è8¯F·7æ6±1‚2é9”41å5¹9¶6å5FªAç7”4±1å5½D“3å5‰9Dä4»B£3é9™9…5è8§7£3é9”41æ6ŒC‰9é9’2®E"2,C 0() 0=D>E 0{B
A 0 0assert.match(app,C 0/Ffunction 0setRunResultReloadBusy\C(busy\C) 0\C{B[B\Cs\CS]D*?Fbutton\C.disabled 0=D 0Boolean\C(busy\C)[B\Cs\CS]D*?Fbutton\C.setAttribute\C("2aria-busy"2,C 0"2true"2\C)[B\Cs\CS]D*?Fbutton\C.textContent 0=D 0"2é9‡7Dæ6–6°0è8¯F»Bå5F–6ç7»B“3æ6EœC"2/F);B
A 0 0const 0loaderStart 0=D 0app.indexOf("2async 0function 0loadTaskRunResult(runId,C 0expectedTaskId) 0{B"2);B
A 0 0const 0loaderEnd 0=D 0app.indexOf("2\C\Cnfunction 0stopTaskPolling()"2,C 0loaderStart);B
A 0 0const 0loader 0=D 0loaderStart 0>E=D 00 0&6&6 0loaderEnd 0>E 0loaderStart 0?F 0app.slice(loaderStart,C 0loaderEnd) 0:A 0"2"2;B
A 0 0assert.match(loader,C 0/Freload-run-result.*disabled[B\Cs\CS]D*?F\C[B"2loading"2,C 0"2retrying"2\C]D[B\Cs\CS]D*?Freturn;B/F);B
A 0 0assert.match(loader,C 0/FsetRunResultReloadBusy\C(true\C)/F);B
A 0 0assert.match(loader,C 0/Ffinally 0\C{B[B\Cs\CS]D*?Fif 0\C(generation 0=D=D=D 0state\C.runResultGeneration\C) 0\C{B[B\Cs\CS]D*?FsetRunResultReloadBusy\C(false\C);B[B\Cs\CS]D*?F\C}D\Cs*\C}D/F);B
A 0 0assert.equal((app.match(/Fstate\C.runResultGeneration 0\C+B=D 01;B\Cs*releaseRunResultReloadBusy\C(\C);B/Fg) 0|C|C 0[B]D).length,C 03,C 0"2æ6–6°0å5»BºAä4»B»Bå5ŠA¡1ã3€01æ61¢2å5¤4Dä4»B»Bå5ŠA¡1å5’2ŒCå5ˆ8†6æ6E0å5œC°0å5D€0å5ˆ8‡7æ6D¢2é9ƒ3½Då5¿F…5é9¡1»Bé9€0šAè8¿F‡7å5®E‰9å5…5¨8å5…5¥5å5F£3è8§7£3é9™9¤4æ6—7§7è8¯F·7æ6±1‚2é9”41"2);B
A}D);B
A
Atest("2watchlist 0marks 0a 0retained 0Run 0result 0when 0re-reading 0fails 0and 0refreshes 0on 0recovery"2,C 0() 0=D>E 0{B
A 0 0assert.ok(app.includes('runResultReadStatus:A 0"2"2'));B
A 0 0assert.ok(app.includes('runResultReadError:A 0"2"2'));B
A 0 0const 0watchlistStart 0=D 0app.indexOf("2function 0renderWatchlist("2);B
A 0 0const 0watchlistEnd 0=D 0app.indexOf("2\C\Cnfunction 0addCurrentSymbolToWatchlist("2,C 0watchlistStart);B
A 0 0const 0watchlist 0=D 0app.slice(watchlistStart,C 0watchlistEnd);B
A 0 0assert.match(watchlist,C 0/FreadState 0=D=D=D 0"2loading"2[B\Cs\CS]D*?Fæ6­D£3å5œC¨8é9‡7Dè8¯F»BïF¼CŒCå5°0šAæ6œCªAç7¡1®Eè8®E¤4/F);B
A 0 0assert.match(watchlist,C 0/FreadState 0=D=D=D 0"2retrying"2[B\Cs\CS]D*?Fé9‡7Dè8¯F•5ä4¸8­DïF¼CŒCå5°0šAæ6œCªAç7¡1®Eè8®E¤4/F);B
A 0 0assert.match(watchlist,C 0/FreadState 0=D=D=D 0"2error"2[B\Cs\CS]D*?Fæ6œC¬Cæ6¬C¡1è8¯F»Bå5F–6å5¤4±1è8´4¥5/F);B
A 0 0const 0resultStart 0=D 0app.indexOf("2function 0renderTaskResult(result) 0{B"2);B
A 0 0const 0resultEnd 0=D 0app.indexOf("2\C\Cnfunction 0"2,C 0resultStart 0+B 010);B
A 0 0assert.match(app.slice(resultStart,C 0resultEnd),C 0/Fstate\C.runResultReadStatus 0=D 0"2"2;B[B\Cs\CS]D*?Fstate\C.runResultReadError 0=D 0"2"2/F);B
A 0 0const 0loaderStart 0=D 0app.indexOf("2async 0function 0loadTaskRunResult(runId,C 0expectedTaskId) 0{B"2);B
A 0 0const 0loaderEnd 0=D 0app.indexOf("2\C\Cnfunction 0stopTaskPolling()"2,C 0loaderStart);B
A 0 0const 0loader 0=D 0app.slice(loaderStart,C 0loaderEnd);B
A 0 0assert.match(watchlist,C 0/FactiveRunReadMatches[B\Cs\CS]D*?Få5½D“3å5‰9D 0Run 0ç7»B“3æ6EœCïF¼CšAæ6­D£3å5œC¨8è8¯F»Bå5F–6æ6œCDå5ŠA¡1ç7«B¯Fç7»B“3æ6EœC/F);B
A 0 0assert.match(watchlist,C 0/Få5½D“3å5‰9D 0Run 0ç7»B“3æ6EœCïF¼CšAæ6šA‚2æ6—7¶6ä4¸8Då5F¯Fè8¯F»BïF¼CŒCæ6­D£3å5œC¨8è8‡7ªAå5ŠA¨8é9‡7Dè8¯F•5/F);B
A 0 0assert.match(watchlist,C 0/Få5½D“3å5‰9D 0Run 0ç7»B“3æ6EœCïF¼CšAè8¯F»Bå5F–6å5¤4±1è8´4¥5/F);B
A 0 0assert.match(loader,C 0/Fstate\C.runResultReadStatus 0=D 0"2loading"2/F);B
A 0 0assert.match(loader,C 0/Fstate\C.runResultReadStatus 0=D 0"2retrying"2[B\Cs\CS]D*?Fif 0\C(state\C.view 0=D=D=D 0"2watchlist"2\C) 0renderWatchlist\C(\C)/F);B
A 0 0assert.match(loader,C 0/Fstate\C.runResultReadStatus 0=D 0"2error"2[B\Cs\CS]D*?Fif 0\C(state\C.view 0=D=D=D 0"2watchlist"2\C) 0renderWatchlist\C(\C)/F);B
A}D);B
A
Atest("2creation,C 0contract 0confirmation,C 0and 0history 0restoration 0use 0distinct 0task 0status 0labels"2,C 0() 0=D>E 0{B
A 0 0const 0createStart 0=D 0app.indexOf("2async 0function 0createTask(event) 0{B"2);B
A 0 0const 0createEnd 0=D 0app.indexOf("2\C\Cnfunction 0startAnalysisForSymbol("2,C 0createStart);B
A 0 0const 0creator 0=D 0app.slice(createStart,C 0createEnd);B
A 0 0assert.ok(creator.includes('setQualityPill($4("2#3task-status-pill"2),C 0"2unknown"2,C 0"2æ6­D£3å5œC¨8å5ˆ8›Bå5»BºA"2);B'));B
A 0 0assert.ok(creator.includes('setQualityPill($4("2#3task-status-pill"2),C 0"2unknown"2,C 0"2æ6­D£3å5œC¨8ç7¡1®Eè8®E¤4"2);B'));B
A 0 0assert.ok(!creator.includes('setQualityPill($4("2#3task-status-pill"2),C 0"2unknown"2,C 0"2æ6­D£3å5œC¨8æ61¢2å5¤4D"2);B'));B
A 0 0const 0restoreStart 0=D 0app.indexOf("2function 0activateTaskReference(record) 0{B"2);B
A 0 0const 0restoreEnd 0=D 0app.indexOf("2\C\Cnfunction 0restoreLastTask("2,C 0restoreStart);B
A 0 0assert.ok(app.slice(restoreStart,C 0restoreEnd).includes('setQualityPill($4("2#3task-status-pill"2),C 0"2unknown"2,C 0"2æ6­D£3å5œC¨8æ61¢2å5¤4D"2);B'));B
A}D);B
A
Atest("2task 0restoration 0and 0manual 0refresh 0mark 0cached 0status 0as 0unconfirmed"2,C 0() 0=D>E 0{B
A 0 0assert.ok(app.includes("2taskStatusLoading:A 0false"2));B
A 0 0const 0pollStart 0=D 0app.indexOf("2async 0function 0pollTask("2);B
A 0 0const 0pollEnd 0=D 0app.indexOf("2\C\Cnasync 0function 0createTask("2,C 0pollStart);B
A 0 0const 0poller 0=D 0app.slice(pollStart,C 0pollEnd);B
A 0 0assert.match(poller,C 0/Ftask 0=D 0candidate;B\Cs*state\C.taskStatusLoading 0=D 0false;B\Cs*state\C.taskStatusError 0=D 0"2"2/F);B
A 0 0assert.match(poller,C 0/Fif 0\C(taskError\C) 0\C{B\Cs*state\C.taskStatusLoading 0=D 0false;B/F);B
A 0 0const 0watchStart 0=D 0app.indexOf("2function 0renderWatchlist("2);B
A 0 0const 0watchEnd 0=D 0app.indexOf("2\C\Cnfunction 0addCurrentSymbolToWatchlist("2,C 0watchStart);B
A 0 0const 0watchlist 0=D 0app.slice(watchStart,C 0watchEnd);B
A 0 0assert.match(watchlist,C 0/Fstate\C.taskStatusLoading 0\C|C\C|C 0Boolean\C(state\C.taskStatusError\C)/F);B
A 0 0assert.match(watchlist,C 0/Fæ6­D£3å5œC¨8ç7¡1®Eè8®E¤4æ6œCDå5ŠA¡1ç7«B¯Fç7ŠA¶6æ6€01ïF¼Cˆ8ä4¸8ŠAæ6¬C¡1ç7ŠA¶6æ6€01ïF¼CšA/F);B
A 0 0const 0manualStart 0=D 0app.indexOf('$4("2#3refresh-task"2).addEventListener("2click"2');B
A 0 0const 0manualEnd 0=D 0app.indexOf('$4("2#3recent-analysis-search"2)',C 0manualStart);B
A 0 0const 0manual 0=D 0app.slice(manualStart,C 0manualEnd);B
A 0 0assert.match(manual,C 0/Fstate\C.taskStatusLoading 0=D 0true;B[B\Cs\CS]D*?FsetQualityPill\C(\C$4\C("2#3task-status-pill"2\C),C 0"2unknown"2,C 0"2æ6­D£3å5œC¨8å5ˆ8·7æ6–6°0"2\C)/F);B
A}D);B
A
Atest("2ç7«B‹Bå5D³3å5ˆ8·7æ6–6°0ç7ŠA¶6æ6€01ä4½D¿Fç7”4¨8å5¿F™9ç7¢2ŒCé9”41é9˜8»Bæ6­D¢2é9‡7Då5¤4D 0Task/Fä4ºA‹Bä4»B¶6è8½D®Eè8¯F¢2"2,C 0() 0=D>E 0{B
A 0 0assert.match(app,C 0/Ffunction 0setTaskRefreshBusy\C(busy\C) 0\C{B[B\Cs\CS]D*?Fbutton\C.disabled 0=D 0Boolean\C(busy\C)[B\Cs\CS]D*?Fbutton\C.setAttribute\C("2aria-busy"2,C 0"2true"2\C)[B\Cs\CS]D*?Fbutton\C.textContent 0=D 0"2ç7«B‹Bå5D³3å5ˆ8·7æ6–6°0ç7ŠA¶6æ6€01"2/F);B
A 0 0const 0manualStart 0=D 0app.indexOf('$4("2#3refresh-task"2).addEventListener("2click"2');B
A 0 0const 0manualEnd 0=D 0app.indexOf('$4("2#3recent-analysis-search"2)',C 0manualStart);B
A 0 0const 0manual 0=D 0manualStart 0>E=D 00 0&6&6 0manualEnd 0>E 0manualStart 0?F 0app.slice(manualStart,C 0manualEnd) 0:A 0"2"2;B
A 0 0assert.match(manual,C 0/Fif 0\C(!state\C.taskId 0\C|C\C|C 0\C$4\C("2#3refresh-task"2\C)\C.disabled\C) 0return/F);B
A 0 0assert.match(manual,C 0/FstopTaskPolling\C(\C);B[B\Cs\CS]D*?FsetTaskRefreshBusy\C(true\C)/F);B
A 0 0assert.match(manual,C 0/FpollTaskSafely\C(generation\C)\C.finally\C(\C(\C) 0=D>E 0\C{B\Cs*if 0\C(generation 0=D=D=D 0state\C.pollGeneration\C) 0\C{B[B\Cs\CS]D*?FsetTaskRefreshBusy\C(false\C);B[B\Cs\CS]D*?F\C}D 0catch 0\C{B/F);B
A 0 0assert.match(app,C 0/Ffunction 0stopTaskPolling\C(\C) 0\C{B[B\Cs\CS]D*?Fstate\C.pollGeneration 0\C+B=D 01;B[B\Cs\CS]D*?Ftry 0\C{B\Cs*setTaskRefreshBusy\C(false\C);B\Cs*\C}D 0catch 0\C{B/F);B
A}D);B
A
A
Atest("2å5ˆ8·7æ6–6°0å5½D“3å5‰9Dé9¡1µ5é9D¢2ä4¼CšAé9šA”4ç7¦6»Bå5ˆ8·7æ6–6°0é9”4™9è8¯F¯Få5¹9¶6å5®E‰9å5…5¨8é9‡7ŠAæ6”4¾Eå5·7¥5å5…5·7æ6 0Fæ6ŒC‰9é9’2®E"2,C 0() 0=D>E 0{B
A 0 0const 0start 0=D 0app.indexOf('$4("2#3refresh-button"2).addEventListener("2click"2,C 0async 0() 0=D>E 0{B');B
A 0 0const 0end 0=D 0app.indexOf('\C\Cn}D);B',C 0start);B
A 0 0const 0handler 0=D 0start 0>E=D 00 0&6&6 0end 0>E 0start 0?F 0app.slice(start,C 0end) 0:A 0"2"2;B
A 0 0assert.ok(handler,C 0"2å5ˆ8·7æ6–6°0æ6ŒC‰9é9’2®Eå5¿F…5é9¡1»Bç7»B‘1å5®EšAå5½D“3å5‰9Dé9¡1µ5é9D¢2å5ˆ8·7æ6–6°0æ6µ51ç7¨8‹B"2);B
A 0 0assert.match(handler,C 0/Flet 0ownsBusyState 0=D 0false/F);B
A 0 0assert.match(handler,C 0/Fif 0\C(button\C?F\C.disabled\C) 0return;B[B\Cs\CS]D*?FownsBusyState 0=D 0true/F);B
A 0 0assert.match(handler,C 0/Fawait 0refreshServices\C(\C)/F);B
A 0 0assert.match(handler,C 0/Ffinally[B\Cs\CS]D*?FownsBusyState[B\Cs\CS]D*?Fbutton\C.disabled 0=D 0false/F);B
A 0 0assert.match(handler,C 0/FappendFailure\C("2å5½D“3å5‰9Dé9¡1µ5é9D¢2å5ˆ8·7æ6–6°0å5¤4±1è8´4¥5ïF¼CšA"2/F);B
A}D);B
Atest("2æ6•5°0æ6D®Eæ6ºA0è8ƒ3½Då5ŠA›Bå5ˆ8·7æ6–6°0å5ˆ8Då5§7‹Bå5ŒC–6æ6ˆ8–6æ6œC€0ç7»Bˆ8è8§7£3é9”41å5¼C‚2å5¸8¸8ä4¸8Dä4¼CšAä4ºA§7ç7”4ŸFæ6œCªAå5¤4„4ç70†6 0rejection"2,C 0() 0=D>E 0{B
A 0 0const 0start 0=D 0app.indexOf("2async 0function 0refreshSources() 0{B"2);B
A 0 0const 0end 0=D 0app.indexOf("2\C\Cnfunction 0shanghaiDateKey("2,C 0start);B
A 0 0const 0refresh 0=D 0start 0>E=D 00 0&6&6 0end 0>E 0start 0?F 0app.slice(start,C 0end) 0:A 0"2"2;B
A 0 0assert.match(refresh,C 0/Flet 0button 0=D 0null;B[B\Cs\CS]D*?Flet 0list 0=D 0null;B[B\Cs\CS]D*?Ftry 0\C{B/F);B
A 0 0assert.match(refresh,C 0/FshowMessage\C("2æ6­D£3å5œC¨8è8¯F»Bå5F–6å5½D“3å5‰9Dè8¿FEæ6E¥5é9…5Dç7½D®Eå5¯F¹9å5ºA”4ç7šA„4æ6•5°0æ6D®Eæ6ºA0è8ƒ3½Då5ŠA›Bâ2€0¦6"2/F);B
A 0 0assert.match(refresh,C 0/Ftry 0\C{B\Cs*if 0\C(button\C) 0\C{B\Cs*button\C.disabled 0=D 0true;B[B\Cs\CS]D*?Fcatch 0\C{B/F);B
A 0 0assert.match(refresh,C 0/Fconst 0payload 0=D 0await 0getDataSources\C(settings\C);B[B\Cs\CS]D*?Fgeneration 0!=D=D 0sourceRefreshGeneration/F);B
A 0 0assert.match(refresh,C 0/Fcatch 0\C(error\C) 0\C{B\Cs*if 0\C(generation 0!=D=D 0sourceRefreshGeneration\C) 0return;B[B\Cs\CS]D*?FshowMessage\C(message,C 0"2empty-row 0error-text"2\C)/F);B
A 0 0assert.match(refresh,C 0/Ffinally 0\C{B[B\Cs\CS]D*?Fif 0\C(generation 0=D=D=D 0sourceRefreshGeneration\C) 0\C{B[B\Cs\CS]D*?Ftry 0\C{B[B\Cs\CS]D*?Fbutton\C.disabled 0=D 0false;B[B\Cs\CS]D*?Fcatch 0\C{B/F);B
A}D);B
A
Atest("2å5·7²2ä4¿FDå5­D˜8è8¿FEæ6E¥5æ6£3€0æ6ŸF¥5ä4¸8Eè8D‰9ç7¨8¿Fæ6£3€0æ6ŸF¥5ç7šA„4å5ˆ8Då5§7‹Bå5ŒC–6/Fè8§7£3é9”41å5¼C‚2å5¸8¸8ä4¸8Dä4¼CšAä4ºA§7ç7”4ŸFæ6œCªAå5¤4„4ç70†6 0rejection"2,C 0() 0=D>E 0{B
A 0 0const 0saved 0=D 0app.match(/Fasync 0function 0checkConnectionFromSettings\C(\C) 0\C{B[B\Cs\CS]D*?F\C\Cn\C}D/F)?F.[B0]D 0|C|C 0"2"2;B
A 0 0const 0draft 0=D 0app.match(/Fasync 0function 0checkDraftConnectionFromSettings\C(\C) 0\C{B[B\Cs\CS]D*?F\C\Cn\C}D/F)?F.[B0]D 0|C|C 0"2"2;B
A 0 0assert.match(saved,C 0/Fgeneration 0!=D=D 0savedConnectionCheckGeneration/F);B
A 0 0assert.match(saved,C 0/Ffinally[B\Cs\CS]D*?FreleaseSettingsCheckButtonSafely/F);B
A 0 0assert.match(draft,C 0/Fgeneration 0!=D=D 0draftConnectionCheckGeneration/F);B
A 0 0assert.match(draft,C 0/Ffinally[B\Cs\CS]D*?FreleaseSettingsCheckButtonSafely/F);B
A 0 0assert.match(draft,C 0/FtestedDraft 0=D 0readConnectionDraft\C(\C)/F);B
A}D);B
Atest("2applySettings 0ç7šA„4å5D•5é9¡1¹9æ6¸8…5ç70†6å5¤4±1è8´4¥5ä4¸8Dä4¼CšAé9˜8»Bæ6–6­Då5…5¶6ä4½D™9é9…5Dç7½D®Eå5ºA”4ç7”4¨8æ6­D¥5é9ªA¤4"2,C 0() 0=D>E 0{B
A 0 0const 0start 0=D 0app.indexOf("2function 0applySettings("2);B
A 0 0const 0end 0=D 0app.indexOf("2\C\Cnfunction 0saveSettingsFromForm("2,C 0start);B
A 0 0const 0apply 0=D 0start 0>E=D 00 0&6&6 0end 0>E 0start 0?F 0app.slice(start,C 0end) 0:A 0"2"2;B
A 0 0assert.ok(apply);B
A 0 0assert.match(apply,C 0/FrunSettingsApplyStep\C("2æ6¸8…5ç70†6æ6—7§7å5ˆ8†6æ6E0ä4»B»Bå5ŠA¡1å5¤4±1è8´4¥5ïF¼CšA"2/F);B
A 0 0assert.match(apply,C 0/FrunSettingsApplyStep\C("2å5¤4±1æ6•5ˆ8å5¸8‚2å5œCºAæ6€0»Bè8§7ˆ8è8¯F·7æ6±1‚2å5¤4±1è8´4¥5ïF¼CšA"2/F);B
A 0 0assert.match(apply,C 0/FrunSettingsApplyStep\C("2å5ˆ8·7æ6–6°0æ6œCDå5ŠA¡1å51¥5å5ºA·7ç7ŠA¶6æ6€01å5¤4±1è8´4¥5ïF¼CšA"2/F);B
A 0 0assert.match(apply,C 0/FreportSettingsApplyFailures\C(failures\C)/F);B
A}D);B
Atest("2ä4»B»Bå5ŠA¡1å5ˆ8‡7æ6D¢2ä4¸8Eå5ˆ8†6æ6E0 0API 0å5ˆ8‡7æ6D¢2ä4¸8Dä4¼CšAè8¢2«Bæ6ŒC‰9é9’2®Eè8§7£3é9”41å5¼C‚2å5¸8¸8ä4¸8­Dæ6–6­D"2,C 0() 0=D>E 0{B
A 0 0assert.match(app,C 0/Ffunction 0releaseRunResultReloadBusy\C(\C) 0\C{B[B\Cs\CS]D*?Ftry 0\C{B\Cs*setRunResultReloadBusy\C(false\C);B\Cs*\C}D 0catch[B\Cs\CS]D*?Fç7»B§7ç7»B­Då5®EŒCæ6ˆ80ä4¸8ŠAä4¸8‹Bæ6–6‡7å5ˆ8‡7æ6D¢2/F);B
A
A 0 0const 0stop 0=D 0app.match(/Ffunction 0stopTaskPolling\C(\C) 0\C{B[B\Cs\CS]D*?F\C\Cn\C}D/F)?F.[B0]D 0|C|C 0"2"2;B
A 0 0assert.ok(stop,C 0"2å51œCæ6­D¢2è8½D®Eè8¯F¢2å5…5±1ç7”4¨8å5‡7½Dæ6•5°0å5¿F…5é9¡1»Bå5­D˜8å5œC¨8"2);B
A 0 0assert.match(stop,C 0/Fstate\C.pollGeneration 0\C+B=D 01;B[B\Cs\CS]D*?Ftry 0\C{B[B\Cs\CS]D*?FsetTaskRefreshBusy\C(false\C);B[B\Cs\CS]D*?Fcatch/F);B
A 0 0assert.match(stop,C 0/Fconst 0timer 0=D 0state\C.pollTimer;B\Cs*state\C.pollTimer 0=D 0null/F);B
A
A 0 0const 0contexts 0=D 0[B
A 0 0 0 0[B"2å5ˆ8›Bå5»BºAä4»B»Bå5ŠA¡1"2,C 0"2async 0function 0createTask(event) 0{B"2,C 0"2function 0startAnalysisForSymbol("2]D,C
A 0 0 0 0[B"2æ61¢2å5¤4Dæ6œC€0è8¿F‘1ä4»B»Bå5ŠA¡1"2,C 0"2function 0activateTaskReference(record) 0{B"2,C 0"2function 0restoreLastTask("2]D,C
A 0 0 0 0[B"2å5ˆ8†6æ6E0 0API 0å5œC°0å5D€0å5ˆ8‡7æ6D¢2"2,C 0"2function 0invalidateActiveTaskForAnalysisBaseChange("2,C 0"2function 0applySettings("2]D
A 0 0]D;B
A 0 0for 0(const 0[Blabel,C 0startMarker,C 0endMarker]D 0of 0contexts) 0{B
A 0 0 0 0const 0start 0=D 0app.indexOf(startMarker);B
A 0 0 0 0const 0end 0=D 0app.indexOf(endMarker,C 0start 0+B 0startMarker.length);B
A 0 0 0 0const 0body 0=D 0start 0>E=D 00 0&6&6 0end 0>E 0start 0?F 0app.slice(start,C 0end) 0:A 0"2"2;B
A 0 0 0 0assert.ok(body.includes("2releaseRunResultReloadBusy();B"2),C 0label 0+B 0"2 0å5ºA”4ä4½D¿Fç7”4¨8ä4¸8Dä4¼CšAå5†6’2æ6³3¡1ç7šA„4 0Run 0æ6ŒC‰9é9’2®Eè8§7£3é9”41"2);B
A 0 0 0 0assert.ok(body.includes("2stopTaskPolling();B"2),C 0label 0+B 0"2 0å5ºA”4å51œCæ6­D¢2æ6—7§7è8½D®Eè8¯F¢2"2);B
A 0 0 0 0assert.doesNotMatch(body,C 0/FsetRunResultReloadBusy\C(false\C);B/F,C 0label 0+B 0"2 0ä4¸8Då5ºA”4ç7›B´4æ6E¥5è8°0ƒ3ç7”4¨8å5F¯Fè8ƒ3½Dä4¸8­Dæ6–6­Dæ6¸8…5ç70†6ç7šA„4æ6ŒC‰9é9’2®Eè8§7£3é9”41"2);B
A 0 0}D
A}D);B
A
Atest("2æ61¢2å5¤4Dæ6œC€0è8¿F‘1ä4»B»Bå5ŠA¡1æ6—7¶6ç7•5ŒCé9D¢2æ6¸8²2æ6ŸF“3å5¤4±1è8´4¥5ä4»BDä4¼CšAå50¯Få5ŠA¨8å5®E‰9å5…5¨8è8½D®Eè8¯F¢2"2,C 0() 0=D>E 0{B
A 0 0const 0start 0=D 0app.indexOf("2function 0activateTaskReference(record) 0{B"2);B
A 0 0const 0end 0=D 0app.indexOf("2\C\Cnfunction 0restoreLastTask("2,C 0start);B
A 0 0const 0activate 0=D 0start 0>E=D 00 0&6&6 0end 0>E 0start 0?F 0app.slice(start,C 0end) 0:A 0"2"2;B
A 0 0const 0taskBinding 0=D 0activate.indexOf("2state.taskId 0=D 0record.task_id;B"2);B
A 0 0const 0uiTry 0=D 0activate.indexOf("2try 0{B"2,C 0taskBinding);B
A 0 0const 0uiCatch 0=D 0activate.indexOf("2catch 0(error)"2,C 0uiTry);B
A 0 0const 0safePoll 0=D 0activate.indexOf("2void 0pollTaskSafely(state.pollGeneration);B"2,C 0uiCatch);B
A 0 0assert.ok(taskBinding 0>E=D 00 0&6&6 0uiTry 0>E 0taskBinding 0&6&6 0uiCatch 0>E 0uiTry,C 0"2å5…5ˆ8ç7»B‘1å5®EšAä4»B»Bå5ŠA¡1ä4¸8ŠAä4¸8‹Bæ6–6‡7ïF¼CŒCå5†6Dä4¿FDæ6ŠA¤4å5ˆ8Då5§7‹Bè8§7†6å5›B¾Eæ6¸8²2æ6ŸF“3"2);B
A 0 0assert.ok(safePoll 0>E 0uiCatch,C 0"2å5D³3ä4½D¿Få5ˆ8Då5§7‹Bè8§7†6å5›B¾Eæ6¸8²2æ6ŸF“3å5¤4±1è8´4¥5ïF¼CŒCä4»BDå5ºA”4å5F‘1è8µ5·7å5®E‰9å5…5¨8è8½D®Eè8¯F¢2"2);B
A 0 0assert.match(activate.slice(uiCatch),C 0/Fæ61¢2å5¤4Dä4»B»Bå5ŠA¡1ç7•5ŒCé9D¢2æ6—7¶6å5F‘1ç7”4ŸFé9”4™9è8¯F¯FïF¼CšA[B\Cs\CS]D*?Fæ6­D£3å5œC¨8ç7»B§7ç7»B­Dè8¯F»Bå5F–6æ6œCDå5ŠA¡1ç7«B¯Fç7ŠA¶6æ6€01[B\Cs\CS]D*?Fvoid 0pollTaskSafely\C(state\C.pollGeneration\C)/F);B
A 0 0assert.match(activate,C 0/Fstate\C.taskId 0=D 0record\C.task_id;B[B\Cs\CS]D*?Fstate\C.taskEvents\C.clear\C(\C);B[B\Cs\CS]D*?Ftry 0\C{B/F);B
A}D);B
A
Atest("2ä4»B»Bå5ŠA¡1è8½D®Eè8¯F¢2æ6„4Få5¤4–6å5¼C‚2å5¸8¸8ä4¼CšAæ6šA‚2å51œCè8‡7ªAå5ŠA¨8å5ˆ8·7æ6–6°0å5¹9¶6ä4¿FDç7•5™9æ6‰9‹Bå5ŠA¨8é9‡7Dè8¯F•5å5…5¥5å5F£3"2,C 0() 0=D>E 0{B
A 0 0const 0safeStart 0=D 0app.indexOf("2function 0pollTaskSafely("2);B
A 0 0const 0safeEnd 0=D 0app.indexOf("2\C\Cnasync 0function 0createTask("2,C 0safeStart);B
A 0 0const 0safePoll 0=D 0safeStart 0>E=D 00 0&6&6 0safeEnd 0>E 0safeStart 0?F 0app.slice(safeStart,C 0safeEnd) 0:A 0"2"2;B
A 0 0assert.match(safePoll,C 0/Freturn 0pollTask\C(generation\C)\C.catch\C(\C(error\C) 0=D>E 0\C{B/F);B
A 0 0assert.match(safePoll,C 0/Fgeneration 0!=D=D 0state\C.pollGeneration 0\C|C\C|C 0!state\C.taskId\C) 0return/F);B
A 0 0assert.match(safePoll,C 0/Fstate\C.taskStatusLoading 0=D 0false;B[B\Cs\CS]D*?Fstate\C.taskStatusError 0=D 0message;B[B\Cs\CS]D*?Fstate\C.taskStatusErrorRetrying 0=D 0false/F);B
A 0 0assert.match(safePoll,C 0/FstopTaskPolling\C(\C);B[B\Cs\CS]D*?Fè8‡7ªAå5ŠA¨8å5ˆ8·7æ6–6°0å5·7²2æ6šA‚2å51œCïF¼CšAå5¤4„4ç70†6ä4»B»Bå5ŠA¡1ç7ŠA¶6æ6€01æ6—7¶6å5F‘1ç7”4ŸFæ6„4Få5¤4–6é9”4™9è8¯F¯F[B\Cs\CS]D*?Fç7‚2¹9å5‡7»Bâ2€0œCç7«B‹Bå5D³3å5ˆ8·7æ6–6°0ç7ŠA¶6æ6€01â2€0Dé9‡7Dè8¯F•5/F);B
A
A 0 0const 0pollStart 0=D 0app.indexOf("2async 0function 0pollTask("2);B
A 0 0const 0poller 0=D 0pollStart 0>E=D 00 0&6&6 0safeStart 0>E 0pollStart 0?F 0app.slice(pollStart,C 0safeStart) 0:A 0"2"2;B
A 0 0assert.doesNotMatch(poller,C 0/FsetTimeout\C(\C(\C) 0=D>E 0pollTask\C(generation\C)/F);B
A 0 0assert.match(poller,C 0/FsetTimeout\C(\C(\C) 0=D>E 0pollTaskSafely\C(generation\C)/F);B
A 0 0assert.ok(app.includes("2void 0pollTaskSafely(state.pollGeneration);B"2),C 0"2æ61¢2å5¤4Dä4»B»Bå5ŠA¡1ä4½D¿Fç7”4¨8å5¼C‚2å5¸8¸8æ6”4¶6æ6•5›Bå5…5¥5å5F£3"2);B
A 0 0assert.ok(app.includes("2await 0pollTaskSafely(generation);B"2),C 0"2å5·7¥5å5…5·7æ6 0Få5ˆ8·7æ6–6°0ä4½D¿Fç7”4¨8å5¼C‚2å5¸8¸8æ6”4¶6æ6•5›Bå5…5¥5å5F£3"2);B
A 0 0assert.ok(app.includes("2void 0pollTaskSafely(generation).finally("2),C 0"2æ6‰9‹Bå5ŠA¨8å5ˆ8·7æ6–6°0ä4½D¿Fç7”4¨8å5¼C‚2å5¸8¸8æ6”4¶6æ6•5›Bå5…5¥5å5F£3å5¹9¶6ä4»BDæ6”4¶6å5°0¾Eå5¿F™9ç7¢2ŒCç7ŠA¶6æ6€01"2);B
A}D);B
A
A
Atest("2è8½D®Eè8¯F¢2å5¼C‚2å5¸8¸8æ6”4¶6æ6•5›Bä4¸8­Dç7šA„4è8¯FŠAæ6–6­Dæ6¸8²2æ6ŸF“3å5¤4±1è8´4¥5ä4¸8Dä4¼CšAå5†6Dæ6¬C¡1æ6‹B’2ç7»BD 0Promise"2,C 0() 0=D>E 0{B
A 0 0const 0safeStart 0=D 0app.indexOf("2function 0pollTaskSafely("2);B
A 0 0const 0safeEnd 0=D 0app.indexOf("2\C\Cnasync 0function 0createTask("2,C 0safeStart);B
A 0 0const 0safePoll 0=D 0safeStart 0>E=D 00 0&6&6 0safeEnd 0>E 0safeStart 0?F 0app.slice(safeStart,C 0safeEnd) 0:A 0"2"2;B
A 0 0assert.ok(safePoll.includes("2return 0pollTask(generation).catch((error) 0=D>E 0{B"2));B
A 0 0assert.ok(safePoll.includes("2generation 0!=D=D 0state.pollGeneration 0|C|C 0!state.taskId"2));B
A 0 0assert.ok(safePoll.includes('appendRecoveryError("2ç7ŠA¶6æ6€01å5¾E½Dæ6 0‡7æ6›B´4æ6–6°0å5¤4±1è8´4¥5ïF¼CšA"2,C 0renderError);B'));B
A 0 0assert.ok(safePoll.includes('appendRecoveryError("2è8½D®Eè8¯F¢2è8¯FŠAæ6–6­Dæ6›B´4æ6–6°0å5¤4±1è8´4¥5ïF¼CšA"2,C 0renderError);B'));B
A 0 0assert.ok(safePoll.includes('appendRecoveryError("2è8‡7ªAé9€0‰9è8‚2¡1å5D¡1ç7‰9‡7å5ˆ8·7æ6–6°0å5¤4±1è8´4¥5ïF¼CšA"2,C 0renderError);B'));B
A 0 0assert.ok(safePoll.includes("2å5¼C‚2å5¸8¸8æ6”4¶6æ6•5›Bä4¸8­Dç7šA„4å5¼C‚2å5¸8¸8ä4¹9ŸFä4¸8Då5¾E—7å5½D¢2æ6ˆ80æ6–6°0ç7šA„4æ6œCªAå5¤4„4ç70†6 0Promise 0rejection"2));B
A 0 0assert.ok(safePoll.includes("2try 0{B
A 0 0 0 0 0 0 0 0stopTaskPolling();B
A 0 0 0 0 0 0}D 0catch 0{B}D"2));B
A}D);B
A
Atest("2æ6‰9‹Bå5ŠA¨8å5ˆ8·7æ6–6°0å5‰9Dç7½D®Eæ6¸8²2æ6ŸF“3å5¼C‚2å5¸8¸8ä4»BDä4¼CšAå5F‘1è8µ5·7å5®E‰9å5…5¨8è8½D®Eè8¯F¢2å5¹9¶6æ6”4¶6å5°0¾Eæ6ŒC‰9é9’2®Eç7ŠA¶6æ6€01"2,C 0() 0=D>E 0{B
A 0 0const 0start 0=D 0app.indexOf('$4("2#3refresh-task"2).addEventListener("2click"2');B
A 0 0const 0end 0=D 0app.indexOf('$4("2#3recent-analysis-search"2)',C 0start);B
A 0 0const 0handler 0=D 0start 0>E=D 00 0&6&6 0end 0>E 0start 0?F 0app.slice(start,C 0end) 0:A 0"2"2;B
A 0 0assert.match(handler,C 0/Ftry 0\C{B[B\Cs\CS]D*?FsetQualityPill[B\Cs\CS]D*?FrenderWatchlist\C(\C);B[B\Cs\CS]D*?Fcatch 0\C(setupError\C)/F);B
A 0 0assert.match(handler,C 0/Fcatch 0\C(setupError\C)[B\Cs\CS]D*?Fä4»BDå5°0†6ç7»B§7ç7»B­Dè8¯F»Bå5F–6æ6œCDå5ŠA¡1ç7«B¯Fç7ŠA¶6æ6€01[B\Cs\CS]D*?Fvoid 0pollTaskSafely\C(generation\C)\C.finally\C(/F);B
A 0 0assert.match(handler,C 0/Fvoid 0pollTaskSafely\C(generation\C)\C.finally\C(\C(\C) 0=D>E 0\C{B\Cs*if 0\C(generation 0=D=D=D 0state\C.pollGeneration\C) 0\C{B\Cs*try 0\C{B\Cs*setTaskRefreshBusy\C(false\C);B\Cs*\C}D 0catch 0\C{B/F);B
A 0 0assert.match(handler,C 0/Ffinally 0ç7šA„4æ6ŒC‰9é9’2®Eè8§7£3é9”41ä4¸8Då5¾E—7å5†6Dæ6¬C¡1å5½D¢2æ6ˆ80æ6œCªAå5¤4„4ç70†6ç7šA„4 0Promise 0rejection/F);B
A}D);B
A
Atest("2Run 0ç7»B“3æ6EœCè8¯F»Bå5F–6å5‡7†6å5¤4‡7é9˜8¶6æ6®Eµ5å5¼C‚2å5¸8¸8ä4¹9ŸFä4¼CšAå5¤4Dä4½DDå5¿F™9ç7¢2ŒCé9”41"2,C 0() 0=D>E 0{B
A 0 0const 0start 0=D 0app.indexOf("2async 0function 0loadTaskRunResult(runId,C 0expectedTaskId) 0{B"2);B
A 0 0const 0end 0=D 0app.indexOf("2\C\Cnfunction 0stopTaskPolling()"2,C 0start);B
A 0 0const 0loader 0=D 0start 0>E=D 00 0&6&6 0end 0>E 0start 0?F 0app.slice(start,C 0end) 0:A 0"2"2;B
A 0 0const 0outerTry 0=D 0loader.indexOf("2try 0{B"2);B
A 0 0const 0busy 0=D 0loader.indexOf("2setRunResultReloadBusy(true);B"2);B
A 0 0const 0setupRender 0=D 0loader.indexOf('if 0(state.view 0=D=D=D 0"2watchlist"2) 0renderWatchlist();B',C 0busy);B
A 0 0const 0whileStart 0=D 0loader.indexOf("2while 0(true)"2,C 0busy);B
A 0 0const 0outerCatch 0=D 0loader.lastIndexOf("2}D 0catch 0(error) 0{B"2);B
A 0 0const 0outerFinally 0=D 0loader.lastIndexOf("2}D 0finally 0{B"2);B
A 0 0assert.ok(outerTry 0>E=D 00 0&6&6 0busy 0>E 0outerTry 0&6&6 0busy 0<C 0setupRender,C 0"2å5¿F™9ç7¢2ŒCé9”41è8®E¾Eç7½D®Eã3€01å5ˆ8Då5§7‹Bå5ŒC–6å5’2ŒCè8‡7ªAé9€0‰9è8‚2¡1é9‡7Dç7»B˜8å5¿F…5é9¡1»Bä4½DDä4ºAEå5¤4–6å5±1‚2 0try 0å5†6…5"2);B
A 0 0assert.ok(whileStart 0>E 0setupRender 0&6&6 0outerCatch 0>E 0whileStart 0&6&6 0outerFinally 0>E 0outerCatch,C 0"2å5¤4–6å5±1‚2 0catch/Ffinally 0å5¿F…5é9¡1»Bå5ŒC…5ä4½DFå5®EŒCæ6•5´4è8¯F»Bå5F–6ç7”4ŸFå5‘1½Då5‘1¨8æ6œCŸF"2);B
A 0 0const 0errorFlow 0=D 0loader.slice(outerCatch);B
A 0 0assert.match(errorFlow,C 0/Fgeneration 0!=D=D 0state\C.runResultGeneration[B\Cs\CS]D*?Fstate\C.taskId 0!=D=D 0expectedTaskId/F);B
A 0 0assert.match(errorFlow,C 0/FdescribeErrorSafely[B\Cs\CS]D*?Ftry 0\C{B\Cs*return 0errorMessage\C(value\C);B\Cs*\C}D 0catch/F);B
A 0 0assert.match(errorFlow,C 0/Fstate\C.runResultReadStatus 0=D 0"2error"2;B[B\Cs\CS]D*?Fè8¯F»Bå5F–6 0Run 0ç7»B“3æ6EœCæ6—7¶6å5F‘1ç7”4ŸFæ6„4Få5¤4–6é9”4™9è8¯F¯F/F);B
A 0 0assert.match(errorFlow,C 0/FappendReadDiagnostic\C("2é9”4™9è8¯F¯Fæ6F0ç7¤4ºAæ6›B´4æ6–6°0å5¤4±1è8´4¥5ïF¼CšA[B\Cs\CS]D*?FappendReadDiagnostic\C("2è8‡7ªAé9€0‰9è8‚2¡1å5D¡1ç7‰9‡7å5ˆ8·7æ6–6°0å5¤4±1è8´4¥5ïF¼CšA/F);B
A 0 0assert.match(errorFlow,C 0/Ffinally 0\C{B[B\Cs\CS]D*?Fif 0\C(generation 0=D=D=D 0state\C.runResultGeneration\C) 0\C{B\Cs*try 0\C{B\Cs*setRunResultReloadBusy\C(false\C);B\Cs*\C}D 0catch 0\C(unlockError\C)/F);B
A 0 0assert.match(errorFlow,C 0/Fæ6œC€0ç7»Bˆ8æ6”4¶6å5°0¾Eæ6œC¬Cè8ºA«Bå5¿F…5é9¡1»Bä4¿FDæ6ŒC1ä4¸8Dæ6ŠA›Bé9”4™9/F);B
A}D);B
A
Atest("2Run 0ç7»B“3æ6EœCç7šA„4å5¼C‚2å5¸8¸8è8¯FŠAæ6–6­Dä4¸8Eæ6ŒC‰9é9’2®Eè8§7£3é9”41å5¤4±1è8´4¥5ä4¸8Dä4¼CšAä4ºA§7ç7”4ŸFä4ºAŒCæ6¬C¡1æ6‹B’2ç7»BD"2,C 0() 0=D>E 0{B
A 0 0const 0start 0=D 0app.indexOf("2async 0function 0loadTaskRunResult(runId,C 0expectedTaskId) 0{B"2);B
A 0 0const 0end 0=D 0app.indexOf("2\C\Cnfunction 0stopTaskPolling()"2,C 0start);B
A 0 0const 0loader 0=D 0start 0>E=D 00 0&6&6 0end 0>E 0start 0?F 0app.slice(start,C 0end) 0:A 0"2"2;B
A 0 0const 0tail 0=D 0loader.slice(loader.lastIndexOf("2}D 0finally 0{B"2));B
A 0 0assert.match(tail,C 0/Fgeneration 0=D=D=D 0state\C.runResultGeneration[B\Cs\CS]D*?Ftry 0\C{B\Cs*setRunResultReloadBusy\C(false\C);B\Cs*\C}D 0catch 0\C(unlockError\C)/F);B
A 0 0assert.match(tail,C 0/Fé9‡7Dæ6–6°0å50¯Fç7”4¨8ç7»B“3æ6EœCè8¯F»Bå5F–6æ6ŒC‰9é9’2®Eå5¤4±1è8´4¥5ïF¼CšA[B\Cs\CS]D*?Fcatch 0\C{B\Cs*\C/F\C/F 0æ6œC€0ç7»Bˆ8æ6”4¶6å5°0¾Eæ6œC¬Cè8ºA«Bå5¿F…5é9¡1»Bä4¿FDæ6ŒC1ä4¸8Dæ6ŠA›Bé9”4™9/F);B
A 0 0assert.match(loader,C 0/Fif 0\C(generation 0!=D=D 0state\C.runResultGeneration 0\C|C\C|C 0state\C.taskId 0!=D=D 0expectedTaskId 0\C|C\C|C[B\Cs\CS]D*?Fstate\C.settings\C.analysisBase 0!=D=D 0analysisBase\C) 0return;B/F);B
A}D);B
A
Atest("2task 0polling 0errors 0mark 0last-known 0status 0stale 0and 0successful 0refresh 0clears 0it"2,C 0() 0=D>E 0{B
A 0 0assert.ok(app.includes('taskStatusError:A 0"2"2'));B
A 0 0assert.ok(app.includes('taskStatusErrorRetrying:A 0false'));B
A 0 0const 0pollStart 0=D 0app.indexOf("2async 0function 0pollTask("2);B
A 0 0const 0pollEnd 0=D 0app.indexOf("2\C\Cnasync 0function 0createTask("2,C 0pollStart);B
A 0 0const 0poller 0=D 0pollStart 0>E=D 00 0&6&6 0pollEnd 0>E 0pollStart 0?F 0app.slice(pollStart,C 0pollEnd) 0:A 0"2"2;B
A 0 0assert.ok(poller.includes("2state.taskStatusError 0=D 0errorMessage(taskError);B"2),C 0"2ä4»B»Bå5ŠA¡1ç7ŠA¶6æ6€01è8¯F»Bå5F–6å5¤4±1è8´4¥5æ6—7¶6ä4¿FDå5­D˜8æ6F0ç7¤4ºA"2);B
A 0 0assert.ok(poller.includes("2state.taskStatusErrorRetrying 0=D 0isRetryableApiError(taskError) 0&6&6 0!taskError.responseContractError;B"2),C 0"2å5ŒCºAå5ˆ8†6è8‡7ªAå5ŠA¨8é9‡7Dè8¯F•5ä4¸8Eæ6°0¸8ä4¹9…5æ6šA‚2å51œC"2);B
A 0 0assert.ok(poller.includes('state.taskStatusError 0=D 0"2"2;B'),C 0"2ä4¸8‹Bä4¸8€0æ6¬C¡1æ6œC‰9æ6•5ˆ8 0Task 0å5›BEå5ŒC…5æ6¸8…5é9™9¤4æ6—7§7é9”4™9è8¯F¯F"2);B
A 0 0assert.ok(poller.includes('setQualityPill($4("2#3task-status-pill"2),C 0"2unknown"2,C 0state.taskStatusErrorRetrying 0?F 0"2ç7ŠA¶6æ6€01å5ˆ8·7æ6–6°0å5¤4±1è8´4¥5"2 0:A 0"2ç7ŠA¶6æ6€01è8¯F»Bå5F–6å5¤4±1è8´4¥5"2)'),C 0"2ä4»B»Bå5ŠA¡1ç7ŠA¶6æ6€01å5¾E½Dæ6 0‡7ä4¸8Då5¾E—7ç7»B§7ç7»B­Då5†6’2å5…5…5ä4¸8ºAå5·7²2ç7¡1®Eè8®E¤4ç7šA„4 0running/Fsucceeded"2);B
A 0 0const 0watchlistRenderer 0=D 0app.slice(app.indexOf("2function 0renderWatchlist("2),C 0app.indexOf("2\C\Cnfunction 0addCurrentSymbolToWatchlist("2));B
A 0 0assert.ok(watchlistRenderer.includes("2ç7ŠA¶6æ6€01è8¯F»Bå5F–6å5¼C‚2å5¸8¸8ïF¼Cˆ8ä4¸8ŠAæ6¬C¡1ç7ŠA¶6æ6€01ïF¼CšA"2),C 0"2è8‡7ªAé9€0‰9å5D¡1ç7‰9‡7å5ºA”4å5°0†6ä4¸8ŠAæ6¬C¡1ç7ŠA¶6æ6€01æ6 0‡7ä4¸8ºAæ6œCªAé9ªAŒCè8¯F1"2);B
A 0 0assert.ok(watchlistRenderer.includes("2è8‡7ªAå5ŠA¨8é9‡7Dè8¯F•5ä4¸8­D"2) 0&6&6 0watchlistRenderer.includes("2è8‡7ªAå5ŠA¨8å5ˆ8·7æ6–6°0å5·7²2æ6šA‚2å51œC"2),C 0"2è8‡7ªAé9€0‰9å5D¡1ç7‰9‡7å5ºA”4å5ŒCºAå5ˆ8†6ä4¸8´4æ6—7¶6é9”4™9è8¯F¯Fä4¸8Eæ6°0¸8ä4¹9…5é9”4™9è8¯F¯F"2);B
A 0 0const 0manualStart 0=D 0app.indexOf('$4("2#3refresh-task"2).addEventListener("2click"2');B
A 0 0const 0manualEnd 0=D 0app.indexOf('$4("2#3recent-analysis-search"2)',C 0manualStart);B
A 0 0const 0manualRefresh 0=D 0app.slice(manualStart,C 0manualEnd);B
A 0 0assert.ok(manualRefresh.includes("2state.taskEventsUnavailable 0=D 0false"2) 0&6&6 0manualRefresh.includes("2pollTaskSafely(generation)"2),C 0"2æ6‰9‹Bå5ŠA¨8å5ˆ8·7æ6–6°0é9‡7Dæ6–6°0å5°0Dè8¯F•5ä4»B»Bå5ŠA¡1ä4¸8Eä4ºA‹Bä4»B¶6è8¯F»Bå5F–6"2);B
A 0 0assert.ok(manualRefresh.includes("2æ6­D£3å5œC¨8æ6‰9‹Bå5ŠA¨8å5ˆ8·7æ6–6°0ä4»B»Bå5ŠA¡1ç7ŠA¶6æ6€01ä4¸8Eé9˜8¶6æ6®Eµ5ä4ºA‹Bä4»B¶6"2),C 0"2æ6‰9‹Bå5ŠA¨8å5ˆ8·7æ6–6°0å5¼C€0å5§7‹Bå50Eå5ºA”4ç7«B‹Bå5D³3å5FDé9¦6ˆ8ïF¼CŒCé91¿Få5…5Dæ6—7§7é9”4™9è8¯F¯Fæ6F0ç7¤4ºAç7œC‹Bè8µ5·7æ6D¥5æ6œCªAå5“3Då5ºA”4"2);B
A}D);B
A
Atest("2å5ˆ8›Bå5»BºAä4»B»Bå5ŠA¡1ç7šA„4å5‰9Dç7½D®E 0UI 0å5¼C‚2å5¸8¸8ä4¹9ŸFä4¼CšAé9‡7ŠAæ6”4¾Eæ6F0ä4ºA¤4é9”41å5’2ŒCæ6ŒC‰9é9’2®E"2,C 0() 0=D>E 0{B
A 0 0const 0start 0=D 0app.indexOf("2async 0function 0createTask(event) 0{B"2);B
A 0 0const 0end 0=D 0app.indexOf("2\C\Cnfunction 0startAnalysisForSymbol("2,C 0start);B
A 0 0const 0creator 0=D 0start 0>E=D 00 0&6&6 0end 0>E 0start 0?F 0app.slice(start,C 0end) 0:A 0"2"2;B
A 0 0const 0declarations 0=D 0creator.indexOf("2let 0button 0=D 0null;B"2);B
A 0 0const 0protectedTry 0=D 0creator.indexOf("2try 0{B"2,C 0declarations);B
A 0 0const 0buttonSetup 0=D 0creator.indexOf('button 0=D 0$4("2#3create-task-button"2);B',C 0protectedTry);B
A 0 0const 0networkCall 0=D 0creator.indexOf("2await 0createAnalysisTask("2,C 0protectedTry);B
A 0 0const 0outerCatch 0=D 0creator.lastIndexOf("2}D 0catch 0(error) 0{B"2);B
A 0 0const 0outerFinally 0=D 0creator.lastIndexOf("2}D 0finally 0{B"2);B
A 0 0assert.ok(declarations 0>E=D 00 0&6&6 0protectedTry 0>E 0declarations 0&6&6 0protectedTry 0<C 0buttonSetup,C 0"2æ6ŒC‰9é9’2®Eæ6ŸF¥5è8¯F¢2ä4¸8Eå5ˆ8Då5§7‹Bå5ŒC–6å5¿F…5é9¡1»Bä4½DDä4ºAEç7»BŸFä4¸8€0 0try 0å5†6…5"2);B
A 0 0assert.ok(protectedTry 0<C 0networkCall 0&6&6 0outerCatch 0>E 0networkCall 0&6&6 0outerFinally 0>E 0outerCatch,C 0"2å5‡7†6å5¤4‡7é9˜8¶6æ6®Eµ5å5’2ŒCç7½D‘1ç7»BœCè8¯F·7æ6±1‚2å5¿F…5é9¡1»Bå5…5±1ä4ºA«Bç7»BŸFä4¸8€0 0catch/Ffinally"2);B
A 0 0assert.match(creator.slice(outerCatch),C 0/Fcatch 0\C(recoveryError\C)[B\Cs\CS]D*?Fæ6—7 0æ6³3•5æ6›B´4æ6–6°0å5EŸFå5§7‹Bé9”4™9è8¯F¯Fæ6F0ç7¤4ºA[B\Cs\CS]D*?Ffinally/F);B
A 0 0assert.match(creator.slice(outerFinally),C 0/Fstate\C.isCreatingTask 0=D 0false;B[B\Cs\CS]D*?Ftry 0\C{B[B\Cs\CS]D*?Fbutton\C.disabled 0=D 0false;B[B\Cs\CS]D*?Fcatch 0\C(unlockError\C)/F);B
A 0 0assert.match(creator.slice(outerFinally),C 0/Ffinally 0ä4¸8­Dç7šA„4å5FDé9¦6ˆ8æ6ˆ8–6 0DOM 0å5¼C‚2å5¸8¸8ä4¸8Då5¾E—7å5½D¢2æ6ˆ80æ6œCªAå5¤4„4ç70†6ç7šA„4 0Promise 0rejection/F);B
A 0 0assert.match(creator,C 0/FcreationContextGeneration 0=D 0state\C.taskContextGeneration/F);B
A 0 0assert.match(creator,C 0/FcreationHistoryGeneration 0=D 0state\C.taskHistoryGeneration/F);B
A}D);B
A
Atest("2new 0task 0creation 0responses 0cannot 0poison 0requested 0symbols 0or 0run/Fstatus 0identity"2,C 0() 0=D>E 0{B
A 0 0const 0createStart 0=D 0app.indexOf("2async 0function 0createTask(event) 0{B"2);B
A 0 0const 0createEnd 0=D 0app.indexOf("2\C\Cnfunction 0startAnalysisForSymbol("2,C 0createStart);B
A 0 0const 0creator 0=D 0createStart 0>E=D 00 0&6&6 0createEnd 0>E 0createStart 0?F 0app.slice(createStart,C 0createEnd) 0:A 0"2"2;B
A 0 0assert.match(creator,C 0/Fconst 0responseSymbolsMatch 0=D 0task\C?F\C.symbols 0=D=D=D 0undefined 0\C|C\C|C[B\Cs\CS]D*?FsameTaskSymbolSet\C(symbols,C 0task\C.symbols\C)/F);B
A 0 0assert.match(creator,C 0/Fconst 0responseRunId 0=D 0responseSymbolsMatch 0&6&6 0responseRunIdValid 0\C?F 0task\C.run_id 0:A 0"2"2/F);B
A 0 0assert.match(creator,C 0/Fconst 0responseStatus 0=D 0responseSymbolsMatch 0&6&6 0responseStatusValid 0\C?F 0task\C.status 0:A 0"2"2/F);B
A 0 0assert.match(creator,C 0/Fconst 0normalizedTask 0=D 0\C{B[B\Cs\CS]D*?Ftask_id:A 0task\C.task_id,C[B\Cs\CS]D*?Frun_id:A 0responseRunId,C[B\Cs\CS]D*?Fstatus:A 0responseStatus,C[B\Cs\CS]D*?Ftask_type:A 0taskType,C[B\Cs\CS]D*?Fsymbols,C/F);B
A 0 0assert.match(creator,C 0/Fstate\C.pollTimer 0=D 0window\C.setTimeout\C(\C(\C) 0=D>E 0pollTaskSafely\C(generation\C),C 0500\C);B/F);B
A 0 0assert.doesNotMatch(creator,C 0/Fvoid 0loadTaskRunResult\C(responseRunId,C 0task\C.task_id\C)/F,C 0"2ç7»Bˆ8æ6€01å5ˆ8›Bå5»BºA/Få5¹9‚2ç7­D‰9å5›BEå5ŒC…5ä4¹9ŸFå5¿F…5é9¡1»Bå5…5ˆ8ç7¡1®Eè8®E¤4é9˜8¶6æ6®Eµ5ä4ºA‹Bä4»B¶6"2);B
A 0 0assert.match(creator,C 0/Fcurrent_stage:A 0responseSymbolsMatch 0\C?F 0task\C.current_stage 0:A 0"2ç7­D‰9å5¾E…5æ6œCDå5ŠA¡1ç7«B¯Fç7¡1®Eè8®E¤4"2/F);B
A}D);B
A
Atest("2late 0task 0creation 0responses 0cannot 0replace 0the 0task 0selected 0while 0submission 0is 0pending"2,C 0() 0=D>E 0{B
A 0 0assert.ok(app.includes("2taskContextGeneration:A 00"2),C 0"2task 0selection 0context 0has 0its 0own 0generation"2);B
A 0 0const 0createStart 0=D 0app.indexOf("2async 0function 0createTask(event) 0{B"2);B
A 0 0const 0createEnd 0=D 0app.indexOf("2\C\Cnfunction 0startAnalysisForSymbol("2,C 0createStart);B
A 0 0const 0creator 0=D 0createStart 0>E=D 00 0&6&6 0createEnd 0>E 0createStart 0?F 0app.slice(createStart,C 0createEnd) 0:A 0"2"2;B
A 0 0assert.match(creator,C 0/Fstate\C.taskContextGeneration 0\C+B=D 01;B\Cs*creationContextGeneration 0=D 0state\C.taskContextGeneration;B/F);B
A 0 0assert.match(creator,C 0/Fif 0\C(creationContextGeneration 0!=D=D 0state\C.taskContextGeneration\C) 0\C{B[B\Cs\CS]D*?FpersistRecentTaskHistory\C(endpointRecord\C)[B\Cs\CS]D*?Få5½D“3å5‰9Dé9€0‰9ä4¸8­Dç7šA„4ä4»B»Bå5ŠA¡1ä4¸8Dä4¼CšAè8¢2«Bè8¦6†6ç7›B–6/F);B
A 0 0assert.match(creator,C 0/Fcatch 0\C(error\C) 0\C{B\Cs*try 0\C{B\Cs*if 0\C(creationContextGeneration 0!=D=D 0state\C.taskContextGeneration\C)/F,C 0"2late 0create 0errors 0must 0not 0hide 0the 0currently 0selected 0task"2);B
A 0 0const 0activateStart 0=D 0app.indexOf("2function 0activateTaskReference(record) 0{B"2);B
A 0 0const 0activateEnd 0=D 0app.indexOf("2\C\Cnfunction 0restoreLastTask("2,C 0activateStart);B
A 0 0assert.match(app.slice(activateStart,C 0activateEnd),C 0/Fstate\C.taskContextGeneration 0\C+B=D 01;B/F);B
A 0 0const 0invalidateStart 0=D 0app.indexOf("2function 0invalidateActiveTaskForAnalysisBaseChange("2);B
A 0 0const 0invalidateEnd 0=D 0app.indexOf("2\C\Cnfunction 0applySettings("2,C 0invalidateStart);B
A 0 0assert.match(app.slice(invalidateStart,C 0invalidateEnd),C 0/Fstate\C.taskContextGeneration 0\C+B=D 01;B/F);B
A}D);B
A
Atest("2polling 0pauses 0on 0permanent 0API 0errors 0and 0Run 0reads 0use 0bounded,C 0stale-safe 0retries"2,C 0() 0=D>E 0{B
A 0 0assert.match(app,C 0/Fif 0\C(taskError 0&6&6 0\C(!isRetryableApiError\C(taskError\C) 0\C|C\C|C 0taskError\C.responseContractError\C)\C) 0\C{B[B\Cs\CS]D*?Fè8‡7ªAå5ŠA¨8å5ˆ8·7æ6–6°0å5·7²2æ6šA‚2å51œC[B\Cs\CS]D*?Freturn;B[B\Cs\CS]D*?Fstate\C.pollTimer 0=D 0window\C.setTimeout\C(\C(\C) 0=D>E 0pollTaskSafely\C(generation\C),C 05000\C);B/F);B
A 0 0assert.match(app,C 0/Fconst 0RUN_RESULT_RETRY_DELAYS_MS 0=D 0\C[B1000,C 02500,C 05000\C]D/F);B
A 0 0assert.match(app,C 0/Fgeneration 0!=D=D 0state\C.runResultGeneration 0\C|C\C|C 0state\C.taskId 0!=D=D 0expectedTaskId 0\C|C\C|C\Cs*endpointGeneration 0!=D=D 0state\C.analysisConnectionGeneration/F);B
A 0 0assert.match(app,C 0/Fstate\C.runResultGeneration 0\C+B=D 01/F);B
A 0 0assert.match(app,C 0/FisRetryableApiError\C(error\C)/F);B
A}D);B
A
A
Atest("2structured 0analysis 0results 0use 0readable 0typography 0and 0mobile-safe 0layouts"2,C 0() 0=D>E 0{B
A 0 0assert.match(stylesheet,C 0/F\C.task-result-metric 0small 0\C{B[B^E}D]D*font-size:A 011px/F);B
A 0 0assert.match(stylesheet,C 0/F\C.task-result-metric 0strong 0\C{B[B^E}D]D*font-size:A 014px/F);B
A 0 0assert.match(stylesheet,C 0/F\C.analysis-result-block 0>E 0h3 0\C{B[B^E}D]D*font-size:A 014px/F);B
A 0 0assert.match(stylesheet,C 0/F\C.analysis-result-fields 0dt 0\C{B[B^E}D]D*font-size:A 011px/F);B
A 0 0assert.match(stylesheet,C 0/F\C.analysis-result-fields 0dd 0\C{B[B^E}D]D*font-size:A 012px/F);B
A 0 0assert.match(stylesheet,C 0/F\C.task-result-actions 0\C.text-button 0\C{B[B^E}D]D*font-size:A 011px/F);B
A 0 0assert.match(stylesheet,C 0/F@0media 0\C(max-width:A 0600px\C)[B\Cs\CS]D*?F\C.analysis-result-grid 0\C{B 0grid-template-columns:A 0minmax\C(0,C 01fr\C);B 0\C}D/F);B
A 0 0assert.match(stylesheet,C 0/Foverflow-wrap:A 0anywhere/F);B
A}D);B
A
A
Atest("2browser-local 0watchlist 0is 0wired 0to 0existing 0query/Fanalysis 0flows 0and 0stores 0only 0references"2,C 0() 0=D>E 0{B
A 0 0for 0(const 0id 0of 0[B"2add-to-watchlist-button"2,C 0"2watchlist-count"2,C 0"2watchlist-feedback"2,C 0"2watchlist-list"2]D) 0{B
A 0 0 0 0assert.match(html,C 0new 0RegExp('id=D"2' 0+B 0id 0+B 0'"2'),C 0"2watchlist 0control 0must 0exist:A 0"2 0+B 0id);B
A 0 0}D
A 0 0assert.match(app,C 0/Ffunction 0renderWatchlist\C(/F);B
A 0 0assert.match(app,C 0/FaddWatchlistSymbol\C(symbol,C 0defaultName,C 0localStorage\C)/F);B
A 0 0assert.match(app,C 0/FremoveWatchlistSymbol\C(symbol,C 0localStorage\C)/F);B
A 0 0assert.match(app,C 0/FrenameWatchlistSymbol\C(record\C.symbol,C 0aliasInput\C.value,C 0localStorage\C)/F);B
A 0 0assert.match(app,C 0/Ffunction 0startAnalysisForSymbol\C(input\C)/F);B
A 0 0assert.match(app,C 0/Ffunction 0startAnalysisForSymbol\C(input\C) 0\C{B[B\Cs\CS]D*?Fsymbol 0=D 0normalizeAnalysisSymbol\C(input\C);B[B\Cs\CS]D*?F\C$4\C("2#3task-symbols"2\C)\C.value 0=D 0symbol;B[B\Cs\CS]D*?FsetView\C("2tasks"2\C);B[B\Cs\CS]D*?F\C$4\C("2#3task-form"2\C)\C.requestSubmit\C(\C);B/F);B
A 0 0assert.match(watchlistModule,C 0/FWATCHLIST_KEY/F);B
A 0 0assert.doesNotMatch(watchlistModule,C 0/FapiKey|CrunResult|Creport\Cs*:A/Fi,C 0"2watchlist 0persistence 0must 0not 0include 0secrets 0or 0report 0payloads"2);B
A 0 0assert.match(stylesheet,C 0/F\C.watchlist-item\Cs*\C{B/F);B
A 0 0assert.match(stylesheet,C 0/F@0media 0\C(max-width:A 0800px\C)[B\Cs\CS]D*?F\C.watchlist-list 0\C{B 0grid-template-columns:A 0minmax\C(0,C 01fr\C);B 0\C}D/F);B
A}D);B
A
A
Atest("2analysis 0tasks 0can 0load 0the 0browser 0watchlist 0and 0withhold 0unavailable 0position 0review"2,C 0() 0=D>E 0{B
A 0 0assert.match(html,C 0/Fid=D"2load-local-watchlist"2/F);B
A 0 0assert.match(html,C 0/Fid=D"2task-symbol-help"2[B^E>E]D*role=D"2status"2/F);B
A 0 0assert.match(html,C 0/F<Coption 0value=D"2watchlist_scan"2>Eè8‡7ªAé9€0‰9è8‚2¡1æ6‰9«Bæ6FFïF¼Cˆ8ä4½D¿Fç7”4¨8ä4¸8‹Bæ6–6¹9æ6¸8…5å5D•5ïF¼C‰9<C\C/Foption>E/F);B
A 0 0assert.match(html,C 0/F<Coption 0value=D"2position_review"2 0disabled>Eæ6ŒC1ä4»B“3å5¤4Dæ6 0¸8ïF¼Cˆ8æ6ŒC1ä4»B“3è8´4¦6æ6œC¬Cæ6E¥5å5…5¥5å50Eå5¼C€0æ6”4¾EïF¼C‰9<C\C/Foption>E/F);B
A 0 0assert.match(app,C 0/Ffunction 0fillTaskSymbolsFromWatchlist\C(\C)[B\Cs\CS]D*?Fconst 0watchlistState 0=D 0safeReadWatchlistState\C(\C);B[B\Cs\CS]D*?Fconst 0records 0=D 0watchlistState\C.items;B[B\Cs\CS]D*?Fif 0\C(!watchlistState\C.storageReadable 0\C|C\C|C 0!watchlistState\C.storageValid\C)[B\Cs\CS]D*?FparseAnalysisSymbols\C(records\C.map\C(\C(record\C) 0=D>E 0record\C.symbol\C)\C.join\C("2,C"2\C)\C)[B\Cs\CS]D*?F\C$4\C("2#3task-type"2\C)\C.value 0=D 0"2watchlist_scan"2/F);B
A 0 0assert.match(app,C 0/F\C$4\C("2#3load-local-watchlist"2\C)\C.addEventListener\C("2click"2,C 0fillTaskSymbolsFromWatchlist\C)/F);B
A 0 0assert.match(serviceWiring,C 0/FVersion:A 0"2unconfigured-no-paper-ledger-v1"2/F);B
A 0 0assert.match(serviceWiring,C 0/FAvailableCash:A 00,C 0TotalEquity:A 00/F);B
A 0 0assert.match(app,C 0/Få5½D“3å5‰9Dæ6²2¡1æ6œC‰9å5F¯Fè8½D½Då5…5¥5ç7šA„4æ6œC¬Cå5œC°0è8‡7ªAé9€0‰9è8‚2¡1/F);B
A}D);B
A
Atest("2daily 0chart 0exposes 0close/Fcandle 0modes,C 0optional 0date 0filters 0and 0adjustment 0basis"2,C 0() 0=D>E 0{B
A 0 0for 0(const 0id 0of 0[B"2bar-start-date"2,C 0"2bar-end-date"2,C 0"2chart-title"2,C 0"2chart-view-close"2,C 0"2chart-view-candles"2,C 0"2chart-legend-label"2]D) 0{B
A 0 0 0 0assert.match(html,C 0new 0RegExp('id=D"2' 0+B 0id 0+B 0'"2'),C 0"2chart/Fdate 0control 0must 0exist:A 0"2 0+B 0id);B
A 0 0}D
A 0 0assert.match(app,C 0/Ffunction 0renderChartView\C(\C)/F);B
A 0 0assert.match(app,C 0/FrenderCandlestickVolumeChart\C(\C$4\C("2#3close-chart"2\C),Cstate\C.bars,CchartOptions\C)/F);B
A 0 0assert.match(app,C 0/Ffunction 0shanghaiDateKey\C(/F);B
A 0 0assert.match(app,C 0/Få5¼C€0å5§7‹Bæ6—7¥5æ6œCŸFä4¸8Dè8ƒ3½Dæ6™9šAä4ºAEç7»B“3æ6DŸFæ6—7¥5æ6œCŸF/F);B
A 0 0assert.match(app,C 0/Fæ6—7¥5 0K 0æ6ŸF¥5è8¯F¢2å5¤4±1è8´4¥5ïF¼CŒCæ6—7§7æ6 0‡7ç7šA„4å5›B¾Eè8¡1¨8å5·7²2æ6¸8…5é9™9¤4/F);B
A 0 0assert.match(stylesheet,C 0/F\C.chart-candle-body/F);B
A 0 0assert.match(stylesheet,C 0/F\C.chart-volume-bar/F);B
A 0 0assert.match(stylesheet,C 0/F\C.chart-view-button\C.is-active/F);B
A}D);B
A
A
Atest("2individual 0stock 0research 0uses 0one 0detail 0workspace 0without 0duplicate 0chip/Fflow 0fetches"2,C 0() 0=D>E 0{B
A 0 0for 0(const 0id 0of 0[B"2research-stock-list-body"2,C 0"2research-overview-panel"2,C 0"2research-overview-price"2,C 0"2research-overview-ma5"2,C 0"2research-overview-quality"2,C 0"2research-open-chart"2,C 0"2research-add-watchlist"2,C 0"2research-start-analysis"2,C 0"2stock-research-panel"2,C 0"2chip-quality"2,C 0"2load-chip-distribution"2,C 0"2capital-flow-quality"2,C 0"2load-capital-flow"2]D) 0{B
A 0 0 0 0assert.ok(html.includes('id=D"2' 0+B 0id 0+B 0'"2'),C 0"2research 0field 0must 0exist:A 0"2 0+B 0id);B
A 0 0}D
A 0 0assert.ok(!html.includes('id=D"2stock-supplemental-panel"2'),C 0"2old 0duplicate 0chip/Fflow 0panel 0must 0be 0gone"2);B
A 0 0assert.ok(app.includes("2function 0renderResearchOverview()"2));B
A 0 0const 0researchOverviewRenderer 0=D 0app.match(/Ffunction 0renderResearchOverview\C(\C) 0\C{B[B\Cs\CS]D*?F\C\Cn\C}D/F)?F.[B0]D 0|C|C 0"2"2;B
A 0 0assert.match(researchOverviewRenderer,C 0/FsameMarketSymbol\C(barsSymbol,C\Cs*symbol\C)/F,C 0"2ç7 0”4ç7©9¶6æ6‘1˜8è8¦61 0MA 0å5¿F…5é9¡1»Bå5¤4Dç7”4¨8 0A 0è8‚2¡1è8¯F1å5ˆ8¸8ä4»B£3ç7 01æ6¯F”4è8¾Eƒ3è8§7„4å5ˆ8™9"2);B
A 0 0assert.doesNotMatch(researchOverviewRenderer,C 0/FbarsSymbol\Cs*=D=D=D\Cs*symbol/F,C 0"2ä4¸8Då5¾E—7ç7”4¨8å5EŸFå5§7‹Bä4»B£3ç7 01å5­D—7ç7¬C¦6ä4¸8²2å5ˆ8¤4æ6–6­Då5D‡7ç7ºA¿Fæ6˜8¯Få50¦6å5±1Eä4ºAEå5½D“3å5‰9Dæ6 0‡7ç7šA„4"2);B
A 0 0assert.ok(app.includes('function 0openResearchStock(symbol,C 0name 0=D 0"2"2)'));B
A 0 0assert.ok(app.includes("2await 0stockResearch.refresh()"2));B
A 0 0assert.ok(!app.includes("2loadSupplementalData"2) 0&6&6 0!app.includes("2resetSupplementPanel"2));B
A 0 0assert.ok(apiClient.includes("2/Fchip-distribution"2) 0&6&6 0apiClient.includes("2/Fcapital-flow"2));B
A 0 0assert.ok(stockResearch.includes("2getChipDistribution"2) 0&6&6 0stockResearch.includes("2getCapitalFlow"2));B
A 0 0assert.ok(stockResearch.includes("2async 0function 0refresh()"2));B
A 0 0assert.ok(stylesheet.includes("2.research-overview-grid"2) 0&6&6 0stylesheet.includes("2.supplement-quality-degraded"2));B
A}D);B
Atest("2restoring 0a 0task 0resets 0the 0previous 0stage-event 0selection"2,C 0() 0=D>E 0{B
A 0 0const 0restoreStart 0=D 0app.indexOf("2function 0activateTaskReference(record) 0{B"2);B
A 0 0const 0restoreEnd 0=D 0app.indexOf("2\C\Cnfunction 0"2,C 0restoreStart 0+B 01);B
A 0 0const 0restoreTask 0=D 0restoreStart 0>E=D 00 0&6&6 0restoreEnd 0>E 0restoreStart 0?F 0app.slice(restoreStart,C 0restoreEnd) 0:A 0"2"2;B
A 0 0assert.ok(restoreTask.includes("2state.taskEvents.clear();B"2),C 0"2æ61¢2å5¤4Dä4»B»Bå5ŠA¡1å5‰9Dæ6¸8…5ç7©9ºAä4¸8ŠAä4¸8€0ä4»B»Bå5ŠA¡1ä4ºA‹Bä4»B¶6"2);B
A 0 0assert.ok(restoreTask.includes("2state.selectedTaskEventSequence 0=D 0null;B"2),C 0"2æ61¢2å5¤4Dä4»B»Bå5ŠA¡1æ6—7¶6æ6¸8…5ç7©9ºAæ6—7§7ä4ºA‹Bä4»B¶6é9€0‰9ä4¸8­Då5ºAFå5F·7"2);B
A 0 0assert.ok(restoreTask.indexOf("2state.selectedTaskEventSequence 0=D 0null;B"2) 0<C 0restoreTask.indexOf("2renderTaskEvents();B"2),C 0"2å5…5ˆ8é9‡7Dç7½D®Eä4ºA‹Bä4»B¶6é9€0‰9æ6‹B©9ïF¼CŒCå5†6Dæ6¸8²2æ6ŸF“3æ6–6°0ä4»B»Bå5ŠA¡1ä4ºA‹Bä4»B¶6"2);B
A}D);B
A
Atest("2watchlist 0labels 0task 0history 0from 0a 0different 0analysis 0API 0address"2,C 0() 0=D>E 0{B
A 0 0assert.ok(app.includes('const 0isOtherAnalysisBase 0=D 0(task) 0=D>E 0Boolean(task 0&6&6 0String(task.analysis_base 0|C|C 0state.settings.analysisBase) 0!=D=D 0String(state.settings.analysisBase));B'));B
A 0 0assert.ok(app.includes('todayLine.textContent 0=D 0"2ä4»BŠAæ6—7¥5å5ˆ8†6æ6E0ïF¼CšAæ6œC‰9æ6œC¬Cå5œC°0è8®E°0å5½D•5"2 0+B 0(todayTaskUsesOtherEndpoint 0?F 0"2ïF¼Cˆ8æ6—7§7å5ˆ8†6æ6E0 0API 0å5œC°0å5D€0ïF¼C‰9"2 0:A 0"2"2)'));B
A 0 0assert.ok(app.includes('(recentTaskUsesOtherEndpoint 0?F 0"2 0Â2·7 0æ6—7§7å5ˆ8†6æ6E0 0API 0å5œC°0å5D€0"2 0:A 0"2"2)'));B
A 0 0const 0todayTaskDeclaration 0=D 0app.indexOf("2const 0todayTask 0=D 0[B...matchingTasks]D"2);B
A 0 0const 0todayEndpointUse 0=D 0app.indexOf("2const 0todayTaskUsesOtherEndpoint 0=D 0isOtherAnalysisBase(todayTask)"2);B
A 0 0assert.ok(todayTaskDeclaration 0>E=D 00 0&6&6 0todayTaskDeclaration 0<C 0todayEndpointUse,C 0"2å5¿F…5é9¡1»Bå5…5ˆ8è8®E¡1ç7®E—7 0todayTask 0å5†6Dè8¯F»Bå5F–6å5…5¶6æ6D¥5æ6ºA0å5œC°0å5D€0ïF¼CŒCé91¿Få5…5Dæ6šA‚2æ6—7¶6æ6€0§7æ6­D»Bå5ŒCºA"2);B
A}D);B
A
Atest("2watchlist 0task 0and 0Run 0summaries 0refresh 0when 0background 0state 0changes"2,C 0() 0=D>E 0{B
A 0 0const 0taskRenderer 0=D 0app.match(/Ffunction 0renderTask\C(task\C) 0\C{B[B\Cs\CS]D*?F\C\Cn\C}D/F)?F.[B0]D 0|C|C 0"2"2;B
A 0 0const 0runRenderer 0=D 0app.match(/Ffunction 0renderTaskResult\C(result\C) 0\C{B[B\Cs\CS]D*?F\C\Cn\C}D/F)?F.[B0]D 0|C|C 0"2"2;B
A 0 0assert.match(taskRenderer,C 0/Fif 0\C(state\C.view 0=D=D=D 0"2watchlist"2\C) 0renderWatchlist\C(\C);B/F,C 0"2Task 0ç7ŠA¶6æ6€01æ6›B´4æ6–6°0æ6—7¶6å5ˆ8·7æ6–6°0å5F¯Fè8§71è8‡7ªAé9€0‰9å5D¡1ç7‰9‡7"2);B
A 0 0assert.match(runRenderer,C 0/Fif 0\C(state\C.view 0=D=D=D 0"2watchlist"2\C) 0renderWatchlist\C(\C);B/F,C 0"2Run 0ç7»B“3æ6EœCæ6›B´4æ6–6°0æ6—7¶6å5ˆ8·7æ6–6°0å5F¯Fè8§71è8‡7ªAé9€0‰9å5D¡1ç7‰9‡7"2);B
A}D);B
A
Atest("2å5¸8‚2å5œCºAæ6€0»Bè8§7ˆ8ä4¸8Eæ6´4»Bè8·7ƒ3å5ºA¦6è8¯F·7æ6±1‚2ç7šA„4å5‡7†6å5¤4‡7é9˜8¶6æ6®Eµ5å5’2ŒC 0finally 0å5¼C‚2å5¸8¸8ä4¼CšAæ6”4¶6æ6•5›B"2,C 0() 0=D>E 0{B
A 0 0assert.match(marketOverview,C 0/Ffunction 0safeOverviewUi\C(operation\C)/F);B
A 0 0assert.match(marketOverview,C 0/Ffunction 0cancel\C(key\C)\C{Bconst 0controller=Dcontrollers\C[Bkey\C]D;Bdelete 0controllers\C[Bkey\C]D;Bgeneration\C[Bkey\C]D\C+B\C+B;Btry\C{Bcontroller\C?F\C.abort\C(\C);B\C}Dcatch\C{B\C}D\C}D/F);B
A 0 0assert.match(marketOverview,C 0/Fasync 0function 0load\C(key\C)\C{B[B\Cs\CS]D*?Ftry\C{B\Cs*cancel\C(key\C);Bseq=Dgeneration\C[Bkey\C]D;Bsettings=Dtypeof 0getSettings/F);B
A 0 0assert.match(marketOverview,C 0/Fasync 0function 0refresh\C(keys=DKEYS,CstatusSelector=D"2#3market-overview-updated"2\C)\C{B[B\Cs\CS]D*?Ftry\C{B[B\Cs\CS]D*?Fawait 0Promise\C.all\C(requested\C.map\C(load\C)\C)[B\Cs\CS]D*?Fcatch\C(error\C)/F);B
A 0 0assert.match(marketOverview,C 0/FObject\C.keys\C(refreshGenerations\C)\C.forEach\C(selector=D>E\C{BrefreshGenerations\C[Bselector\C]D\C+B=D1;B\C}D\C);B[B\Cs\CS]D*?FKEYS\C.forEach/F);B
A 0 0assert.match(marketActivity,C 0/Ffunction 0safeActivityUi\C(operation\C)/F);B
A 0 0assert.match(marketActivity,C 0/Fasync 0function 0loadDragon\C(\C{Bappend=Dfalse\C}D=D\C{B\C}D\C)\C{B[B\Cs\CS]D*?Ftry\C{B\Cs*abort\C("2dragon"2\C);Bseq=Dgeneration\C.dragon;Bcfg=Dsettings\C(\C)/F);B
A 0 0assert.match(marketActivity,C 0/Ffinally\C{B[B\Cs\CS]D*?Fif\C(seq!=D=Dnull&6&6generation\C.dragon=D=D=Dseq\C)[B\Cs\CS]D*?FsafeActivityUi\C(\C(\C)=D>E\C{Bconst 0next=D\C$4\C("2#3dragon-tiger-next"2\C)/F);B
A 0 0assert.match(marketActivity,C 0/Fasync 0function 0loadCalendar\C(\C)\C{B[B\Cs\CS]D*?Ftry\C{B\Cs*abort\C("2calendar"2\C);Bseq=Dgeneration\C.calendar;Bcfg=Dsettings\C(\C)/F);B
A 0 0assert.match(marketActivity,C 0/Ffinally\C{B[B\Cs\CS]D*?Fif\C(seq!=D=Dnull&6&6generation\C.calendar=D=D=Dseq\C)safeActivityUi/F);B
A}D);B
A
Atest("2market 0overview 0refresh 0summaries 0are 0isolated 0by 0status 0region 0and 0guarded 0against 0stale 0runs"2,C 0() 0=D>E 0{B
A 0 0assert.match(marketOverview,C 0/FrefreshGenerations=DObject\C.create\C(null\C)/F);B
A 0 0assert.match(marketOverview,C 0/Fconst 0currentGeneration=D\C(refreshGenerations\C[BstatusSelector\C]D\C|C\C|C0\C)\C+B1/F);B
A 0 0assert.match(marketOverview,C 0/FrefreshGenerations\C[BstatusSelector\C]D=DcurrentGeneration/F);B
A 0 0assert.match(marketOverview,C 0/Fif\C(currentGeneration!=D=DrefreshGenerations\C[BstatusSelector\C]D\C)return/F);B
A}D);B
A
Atest("2market 0connection 0changes 0invalidate 0all 0market 0overview 0requests 0and 0old 0summaries"2,C 0() 0=D>E 0{B
A 0 0const 0start 0=D 0marketOverview.indexOf("2function 0invalidateConnection(){B"2);B
A 0 0const 0end 0=D 0marketOverview.indexOf("2\C\Cn 0}D"2,C 0start);B
A 0 0const 0body 0=D 0start 0>E=D 00 0&6&6 0end 0>E 0start 0?F 0marketOverview.slice(start,C 0end) 0:A 0"2"2;B
A 0 0assert.ok(body.includes("2KEYS.forEach(key=D>E{B"2));B
A 0 0assert.ok(body.includes("2safeOverviewUi(()=D>Ecancel(key));B"2),C 0"2è8¿FEæ6E¥5å5F˜8æ6›B´4å5ºA”4é9€0šAè8¿F‡7å5®E‰9å5…5¨8å5…5¥5å5F£3ä4¸8­Dæ6­D¢2å50„4å5¸8‚2å5œCºAæ6¨8¡1å5D—7è8¯F·7æ6±1‚2å5¹9¶6æ6E¨8è8¿F›B 0generation"2);B
A 0 0assert.ok(body.includes('safeOverviewUi(()=D>Epending(key,C"2è8¡1ŒCæ6ƒ3…5è8¿FEæ6E¥5é9…5Dç7½D®Eå5·7²2å5F˜8æ6›B´4ïF¼CŒCæ6—7§7ç7»B“3æ6EœCå5·7²2æ6¸8…5é9™9¤4ïF¼CŒCè8¯F·7é9‡7Dæ6–6°0å5ˆ8·7æ6–6°0ã3€0‚2"2,C"2warning"2));B'));B
A 0 0assert.ok(body.includes("2refreshGenerations[Bselector]D+B=D1;B"2),C 0"2ä4½DœCå5ºAŸFæ6—7§7å5ˆ8·7æ6–6°0æ6‘1˜8è8¦61å5›BEå5†6™9"2);B
A 0 0assert.ok(body.indexOf("2Object.keys(refreshGenerations)"2) 0<C 0body.indexOf("2KEYS.forEach(key=D>E{B"2),C 0"2å5¿F…5é9¡1»Bå5…5ˆ8ä4½D¿Fæ6‰9€0æ6œC‰9æ6—7§7æ6±1‡7æ6€0»Bå5“3Då5ºA”4å5¤4±1æ6•5ˆ8ïF¼CŒCå5†6Dé9€00é9¡1¹9æ6¸8…5ç70†6æ6¨8¡1å5D—7"2);B
A 0 0assert.ok(body.includes('"2#3market-overview-updated"2,C"2#3market-data-updated"2'));B
A 0 0assert.ok(marketOverview.includes("2return 0{Brefresh,CinvalidateConnection}D;B"2));B
A 0 0const 0settingsStart 0=D 0app.indexOf("2function 0applySettings(nextSettings,C"2);B
A 0 0const 0settingsEnd 0=D 0app.indexOf("2\C\Cnfunction 0saveSettingsFromForm"2,C 0settingsStart);B
A 0 0const 0settingsApplication 0=D 0settingsStart 0>E=D 00 0&6&6 0settingsEnd 0>E 0settingsStart 0?F 0app.slice(settingsStart,C 0settingsEnd) 0:A 0"2"2;B
A 0 0assert.ok(settingsApplication.includes("2marketOverview.invalidateConnection()"2),C 0"2è8¿FEæ6E¥5å5F˜8æ6›B´4é9€0šAè8¿F‡7å5®E‰9å5…5¨8æ6­D¥5é9ªA¤4å5¤4±1æ6•5ˆ8é9¦6–6é9¡1µ5è8¯F·7æ6±1‚2"2);B
A}D);B
A
Atest("2market 0connection 0changes 0invalidate 0in-flight 0dragon-tiger 0and 0calendar 0requests"2,C 0() 0=D>E 0{B
A 0 0const 0invalidateStart 0=D 0marketActivity.indexOf("2function 0invalidateConnection() 0{B"2);B
A 0 0const 0invalidateEnd 0=D 0marketActivity.indexOf("2\C\Cn 0}D"2,C 0invalidateStart);B
A 0 0const 0invalidateBody 0=D 0invalidateStart 0>E=D 00 0&6&6 0invalidateEnd 0>E 0invalidateStart
A 0 0 0 0?F 0marketActivity.slice(invalidateStart,C 0invalidateEnd)
A 0 0 0 0:A 0"2"2;B
A 0 0assert.ok(invalidateBody.includes('safeActivityUi(()=D>Eabort("2dragon"2))'),C 0"2è8¿FEæ6E¥5é9…5Dç7½D®Eå5F˜8æ6›B´4å5ºA”4é9€0šAè8¿F‡7å5®E‰9å5…5¨8å5…5¥5å5F£3ä4¸8­Dæ6­D¢2é9¾E™9è8™9Eæ6¦6œCè8¯F·7æ6±1‚2"2);B
A 0 0assert.ok(invalidateBody.includes('safeActivityUi(()=D>Eabort("2calendar"2))'),C 0"2è8¿FEæ6E¥5é9…5Dç7½D®Eå5F˜8æ6›B´4å5ºA”4é9€0šAè8¿F‡7å5®E‰9å5…5¨8å5…5¥5å5F£3ä4¸8­Dæ6­D¢2ä4ºA¤4æ6˜8“3æ6—7¥5å5E†6è8¯F·7æ6±1‚2"2);B
A 0 0assert.ok(invalidateBody.includes('$4("2#3dragon-tiger-body"2)'),C 0"2å5ºA”4æ6¸8…5é9™9¤4æ6—7§7é9¾E™9è8™9Eæ6¦6œCç7»B“3æ6EœC"2);B
A 0 0assert.ok(invalidateBody.includes('$4("2#3trading-calendar-grid"2).replaceChildren'),C 0"2å5ºA”4æ6¸8…5é9™9¤4æ6—7§7ä4ºA¤4æ6˜8“3æ6—7¥5å5E†6ç7»B“3æ6EœC"2);B
A 0 0assert.ok(marketActivity.includes("2return 0{Brefresh:A()=D>EPromise.all([BloadDragon(),CloadCalendar()]D),CinvalidateConnection}D;B"2));B
A 0 0const 0settingsStart 0=D 0app.indexOf("2function 0applySettings(nextSettings,C"2);B
A 0 0const 0settingsEnd 0=D 0app.indexOf("2\C\Cnfunction 0saveSettingsFromForm"2,C 0settingsStart);B
A 0 0const 0settingsApplication 0=D 0settingsStart 0>E=D 00 0&6&6 0settingsEnd 0>E 0settingsStart
A 0 0 0 0?F 0app.slice(settingsStart,C 0settingsEnd)
A 0 0 0 0:A 0"2"2;B
A 0 0assert.ok(settingsApplication.includes("2marketActivity.invalidateConnection()"2),C 0"2è8¿FEæ6E¥5å5F˜8æ6›B´4é9€0šAè8¿F‡7å5®E‰9å5…5¨8æ6­D¥5é9ªA¤4å5¤4±1æ6•5ˆ8é9¾E™9è8™9Eæ6¦6œCå5’2ŒCä4ºA¤4æ6˜8“3æ6—7¥5å5E†6è8¯F·7æ6±1‚2"2);B
A}D);B
A
Atest("2an 0older 0stock-research 0load 0cannot 0unlock 0the 0current 0stock's 0loading 0button"2,C 0() 0=D>E 0{B
A 0 0assert.match(stockResearch,C 0/FloadResearchGeneration:A0/F);B
A 0 0assert.match(stockResearch,C 0/Ffunction 0reset\C(symbol\C)\C{B[B\Cs\CS]D*?Fstate\C.loadResearchGeneration\C+B=D1;B[B\Cs\CS]D*?F\C$4\C("2#3load-stock-research"2\C)/F);B
A 0 0assert.match(stockResearch,C 0/Fasync 0function 0loadResearch\C(\C)\C{B[B\Cs\CS]D*?FrequestGeneration=D\C+B\C+Bstate\C.loadResearchGeneration;B[B\Cs\CS]D*?Fawait 0Promise\C.allSettled\C([B\Cs\CS]D*?FrequestGeneration!=D=Dstate\C.loadResearchGeneration\C|C\C|Csymbol!=D=Dstate\C.symbol[B\Cs\CS]D*?Ffinally\C{B[B\Cs\CS]D*?FrequestGeneration=D=D=Dstate\C.loadResearchGeneration&6&6symbol=D=D=Dstate\C.symbol/F);B
A}D);B
A
Atest("2stale 0stock 0research 0detail 0requests 0cannot 0unlock 0buttons 0for 0a 0different 0symbol"2,C 0() 0=D>E 0{B
A 0 0assert.match(stockResearch,C 0/Ffunction 0reset\C(symbol\C)[B\Cs\CS]D*?F\C[B"2#3load-valuations"2,C"2è8¯F»Bå5F–6ä4¼C°0å5€0¼Cæ6ŒC‡7æ6 0‡7"2\C]D[B\Cs\CS]D*?F\C[B"2#3load-chip-distribution"2,C"2è8¯F»Bå5F–6ç7­D¹9ç7 01å5ˆ8†6å5¸8ƒ3"2\C]D[B\Cs\CS]D*?F\C[B"2#3load-capital-flow"2,C"2è8¯F»Bå5F–6è8µ5„4é9‡7‘1æ6µ51"2\C]D/F);B
A 0 0assert.match(stockResearch,C 0/Fasync 0function 0runSupplementRequest\C(buttonId,CidleText,Ckey,Crequest,Crender,CsuccessMessage\C)[B\Cs\CS]D*?Fconst 0operation=DfetchOne\C(key,Crequest,Crender\C)[B\Cs\CS]D*?Fgeneration=Dstate\C.generation\C[Bkey\C]D[B\Cs\CS]D*?Fgeneration=D=D=Dstate\C.generation\C[Bkey\C]D&6&6symbol=D=D=Dstate\C.symbol/F);B
A 0 0assert.match(stockResearch,C 0/Fasync 0function 0loadValuations\C(\C)\C{B[B\Cs\CS]D*?FrunSupplementRequest\C("2#3load-valuations"2/F);B
A 0 0assert.match(stockResearch,C 0/Fasync 0function 0loadChipDistribution\C(\C)\C{B[B\Cs\CS]D*?FrunSupplementRequest\C("2#3load-chip-distribution"2/F);B
A 0 0assert.match(stockResearch,C 0/Fasync 0function 0loadCapitalFlow\C(\C)\C{B[B\Cs\CS]D*?FrunSupplementRequest\C("2#3load-capital-flow"2/F);B
A}D);B
A
Atest("2stock 0research 0clears 0stale 0snapshots 0on 0reload,C 0request 0errors,C 0and 0missing 0API 0key"2,C 0() 0=D>E 0{B
A 0 0assert.match(stockResearch,C 0/Ffunction 0clearPanel\C(/F);B
A 0 0assert.match(stockResearch,C 0/Ffunction 0markApiKeyRequired\C(/F);B
A 0 0assert.match(stockResearch,C 0/Ffunction 0invalidate\C(/F);B
A 0 0assert.match(stockResearch,C 0/FclearPanel\C(key,C"2æ6­D£3å5œC¨8è8¯F»Bå5F–6æ6œC¬Cæ6¬C¡1è8¯F·7æ6±1‚2ïF¼CŒCå5°0šAæ6—7 0æ6–6°0ç7»B“3æ6EœCã3€0‚2"2/F);B
A 0 0assert.match(stockResearch,C 0/FsetQuality\C(panel\C.quality,C"2invalid"2,C"2è8¯F»Bå5F–6å5¤4±1è8´4¥5"2\C)/F);B
A 0 0assert.match(stockResearch,C 0/Fif\C(!settings\C?F\C.apiKey\C)\C{BmarkApiKeyRequired\C(\C);Breturn 0false;B\C}D/F);B
A 0 0assert.match(stockResearch,C 0/FclearPanel\C("2announcements"2,C"2æ6­D£3å5œC¨8è8¯F»Bå5F–6æ6œC¬Cæ6¬C¡1å5…5¬Cå5‘1ŠAïF¼CŒCå5°0šAæ6—7 0æ6–6°0ç7»B“3æ6EœCã3€0‚2"2/F);B
A 0 0assert.match(stockResearch,C 0/Fvaluations-reasons/F);B
A 0 0assert.match(app,C 0/FstockResearch\C.invalidate\C(/F);B
A 0 0assert.match(stockResearch,C 0/Fif\C(!append\C)setQuality\C("2announcements-quality"2,C"2invalid"2,C"2è8¯F»Bå5F–6å5¤4±1è8´4¥5"2\C)/F);B
A 0 0assert.match(stockResearch,C 0/Fconst 0replacingInFlight=Dstate\C.announcementLoading;B[B\Cs\CS]D*?Fif\C(replacingInFlight&6&6append\C)return 0false;B[B\Cs\CS]D*?Fif\C(replacingInFlight\C)\C{Babort\C("2announcements"2\C);Bstate\C.announcementLoading=Dfalse;B\C}D/F);B
A 0 0assert.match(stockResearch,C 0/Fif\C(replacingInFlight&6&6append\C)return 0false;B[B\Cs\CS]D*?Fif\C(replacingInFlight\C)\C{Babort\C("2announcements"2\C);Bstate\C.announcementLoading=Dfalse;B\C}D\Cs*else 0abort\C("2announcements"2\C);B\Cs*seq=Dstate\C.generation\C.announcements/F);B
A}D);B
A
Atest("2changing 0market 0API 0credentials 0invalidates 0old 0stock-search 0suggestions"2,C 0() 0=D>E 0{B
A 0 0assert.match(app,C 0/Ffunction 0applySettings\C(nextSettings,C 0\C{B 0persistPreviousActiveTask 0=D 0true 0\C}D 0=D 0\C{B\C}D\C) 0\C{B[B\Cs\CS]D*?FpreviousSettings\C.apiKey 0!=D=D 0nextSettings\C.apiKey 0\C|C\C|C\Cs*previousSettings\C.marketBase 0!=D=D 0nextSettings\C.marketBase[B\Cs\CS]D*?FrunSettingsApplyStep\C("2å5F–6æ6¶6ˆ8è8‚2¡1ç7¥5¨8æ60œCç7´4¢2è8¯F·7æ6±1‚2å5¤4±1è8´4¥5ïF¼CšA"2,C 0\C(\C) 0=D>E 0cancelStockSearchRequest\C(\C),C 0failures\C);B[B\Cs\CS]D*?Fstate\C.stockSearchResults 0=D 0\C[B\C]D;B[B\Cs\CS]D*?FrunSettingsApplyStep\C("2æ6¸8…5ç70†6è8‚2¡1ç7¥5¨8æ60œCç7´4¢2ç7»B“3æ6EœCå5¤4±1è8´4¥5ïF¼CšA"2,C 0\C(\C) 0=D>E 0\C$4\C("2#3stock-search-results"2\C)\C.replaceChildren\C(\C),C 0failures\C);B[B\Cs\CS]D*?FrunSettingsApplyStep\C("2å5…5³3é9—7­Dè8‚2¡1ç7¥5¨8æ60œCç7´4¢2é9D¢2æ6D¿Få5¤4±1è8´4¥5ïF¼CšA"2,C 0\C(\C) 0=D>E 0hideStockSearch\C(\C),C 0failures\C);B/F);B
A 0 0assert.match(app,C 0/Ffunction 0cancelStockSearchRequest\C(\C)[B\Cs\CS]D*?Fstate\C.stockSearchGeneration 0\C+B=D 01;B/F);B
A}D);B
A
Atest("2recent-task 0suppression 0and 0migration 0are 0scoped 0to 0the 0analysis 0API 0base"2,C 0() 0=D>E 0{B
A 0 0assert.match(app,C 0/FsuppressedHistoryTaskBase:A 0"2"2/F);B
A 0 0assert.match(app,C 0/Fstate\C.suppressedHistoryTaskId 0=D=D=D 0state\C.taskId 0&6&6 0state\C.suppressedHistoryTaskBase 0=D=D=D 0taskAnalysisBase/F);B
A 0 0assert.match(app,C 0/FhistoryItem 0=D 0safeReadRecentTasks\C(\C)\C.find\C(\C(item\C) 0=D>E\Cs*item\C.task_id 0=D=D=D 0record\C.task_id 0&6&6\Cs*String\C(item\C.analysis_base 0\C|C\C|C 0taskAnalysisBase\C) 0=D=D=D 0taskAnalysisBase/F);B
A 0 0assert.match(app,C 0/Fanalysis_base:A 0historyItem\C.analysis_base 0\C|C\C|C 0taskAnalysisBase/F);B
A 0 0assert.match(app,C 0/Fstate\C.suppressedHistoryTaskBase 0=D 0state\C.taskAnalysisBase 0\C|C\C|C 0state\C.settings\C.analysisBase/F);B
A}D);B
A
Atest("2task 0polling 0and 0saved 0task 0references 0are 0scoped 0to 0the 0analysis 0API 0base"2,C 0() 0=D>E 0{B
A 0 0assert.match(app,C 0/FtaskAnalysisBase:A 0"2"2/F);B
A 0 0assert.match(app,C 0/FanalysisConnectionGeneration:A 00/F);B
A 0 0assert.match(app,C 0/Ffunction 0invalidateActiveTaskForAnalysisBaseChange\C(/F);B
A 0 0assert.match(app,C 0/Fif 0\C(analysisBaseChanged\C) 0\C{B[B\Cs\CS]D*?FinvalidateActiveTaskForAnalysisBaseChange\C(/F);B
A 0 0assert.match(app,C 0/FendpointGeneration 0!=D=D 0state\C.analysisConnectionGeneration/F);B
A 0 0assert.match(app,C 0/Fconst 0taskAnalysisBase 0=D 0state\C.taskAnalysisBase 0\C|C\C|C 0state\C.settings\C.analysisBase;B[B\Cs\CS]D*?Fanalysis_base:A 0taskAnalysisBase/F);B
A 0 0assert.match(app,C 0/Fæ6—7§7ä4»B»Bå5ŠA¡1ä4¸8Då5†6Dè8‡7ªAå5ŠA¨8è8½D®Eè8¯F¢2/F);B
A 0 0const 0baseInvalidation 0=D 0app.match(/Ffunction 0invalidateActiveTaskForAnalysisBaseChange\C([B\Cs\CS]D*?F\C\Cn\C}D/F)?F.[B0]D 0|C|C 0"2"2;B
A 0 0assert.match(baseInvalidation,C 0/FsafelyUpdateTaskContextUi\C("2é9‡7Dç7»B˜8è8‡7ªAé9€0‰9è8‚2¡1å5D¡1ç7‰9‡7å5¤4±1è8´4¥5ïF¼CšA"2,C 0\C(\C) 0=D>E 0renderWatchlist\C(\C)\C)/F,C 0"2å5ˆ8‡7æ6D¢2å5ˆ8†6æ6E0å5œC°0å5D€0å50Eå5ºA”4å5œC¨8å5®E‰9å5…5¨8æ6¸8…5ç70†6æ6­D¥5é9ªA¤4ä4¸8­Dé9‡7Dç7»B˜8è8‡7ªAé9€0‰9å5D¡1ç7‰9‡7"2);B
A 0 0assert.match(app,C 0/Fif 0\C(requestedBase 0!=D=D 0state\C.settings\C.analysisBase\C)/F);B
A 0 0assert.match(recentTaskModule,C 0/Fanalysis_base:A 0typeof 0record\C.analysis_base 0=D=D=D 0"2string"2/F);B
A}D);B
A
Atest("2topbar 0service 0health 0checks 0have 0bounded 0timeouts 0and 0pass 0abort 0signals"2,C 0() 0=D>E 0{B
A 0 0assert.ok(healthCheck.includes("2export 0async 0function 0runHealthCheck(check,C 0settings,C 0timeoutMs 0=D 06000)"2));B
A 0 0assert.ok(healthCheck.includes("2controller.abort()"2));B
A 0 0assert.match(app,C 0/Fasync 0function 0refreshServices\C(\C)[B\Cs\CS]D*?FPromise\C.all\C(\C[B[B\Cs\CS]D*?FrunHealthCheck\C(marketHealth,C 0settings\C)[B\Cs\CS]D*?FrunHealthCheck\C(analysisHealth,C 0settings\C)/F);B
A}D);B
A
Atest("2data-center 0checks 0are 0invalidated 0when 0a 0saved 0API 0connection 0changes"2,C 0() 0=D>E 0{B
A 0 0assert.match(dataCenter,C 0/Flet 0generation 0=D 00;B/F);B
A 0 0assert.match(dataCenter,C 0/Fif 0\C(current 0!=D=D 0generation\C) 0return;B/F);B
A 0 0assert.match(dataCenter,C 0/Ffunction 0invalidate\C(message 0=D 0"2è8¿FEæ6E¥5é9…5Dç7½D®Eå5·7²2å5F˜8æ6›B´4ïF¼CŒCä4¹9‹Bå5‰9Dç7šA„4æ6£3€0æ6ŸF¥5ç7»B“3æ6EœCå5·7²2ä4½DœCå5ºAŸFã3€0‚2è8¯F·7é9‡7Dæ6–6°0æ6£3€0æ6ŸF¥5ã3€0‚2"2\C) 0\C{B[B\Cs\CS]D*?Fgeneration 0\C+B=D 01;B[B\Cs\CS]D*?Fchecking 0=D 0false;B[B\Cs\CS]D*?FsetBadge\C("2#3data-center-summary-status"2,C 0"2unknown"2,C 0"2é9œC€0é9‡7Dæ6–6°0æ6£3€0æ6ŸF¥5"2\C)/F);B
A 0 0assert.match(dataCenter,C 0/Ffinally 0\C{B[B\Cs\CS]D*?Fif 0\C(current 0=D=D=D 0generation\C) 0\C{B[B\Cs\CS]D*?Fchecking 0=D 0false;B/F);B
A 0 0assert.match(dataCenter,C 0/Freturn 0\C{B 0refresh,C 0invalidate 0\C}D;B/F);B
A 0 0assert.match(app,C 0/Fconst 0marketConnectionChanged 0=D 0previousSettings\C.apiKey 0!=D=D 0nextSettings\C.apiKey 0\C|C\C|C\Cs*previousSettings\C.marketBase 0!=D=D 0nextSettings\C.marketBase;B[B\Cs\CS]D*?Fif 0\C(analysisBaseChanged 0\C|C\C|C 0marketConnectionChanged\C) 0\C{B[B\Cs\CS]D*?FrunSettingsApplyStep\C("2å5¤4±1æ6•5ˆ8æ6•5°0æ6D®Eä4¸8­Då5¿Fƒ3æ6£3€0æ6ŸF¥5ç7»B“3æ6EœCå5¤4±1è8´4¥5ïF¼CšA"2,C 0\C(\C) 0=D>E\Cs*dataCenter\C.invalidate\C(/F);B
A}D);B
A
Atest("2connection-check 0results 0are 0isolated 0between 0saved 0settings 0and 0unsaved 0draft"2,C 0() 0=D>E 0{B
A 0 0const 0saved 0=D 0app.match(/Fasync 0function 0checkConnectionFromSettings\C(\C) 0\C{B[B\Cs\CS]D*?F\C\Cn\C}D/F)?F.[B0]D 0|C|C 0"2"2;B
A 0 0const 0draft 0=D 0app.match(/Fasync 0function 0checkDraftConnectionFromSettings\C(\C) 0\C{B[B\Cs\CS]D*?F\C\Cn\C}D/F)?F.[B0]D 0|C|C 0"2"2;B
A 0 0assert.match(saved,C 0/Fconst 0generation 0=D 0\C+B\C+BsavedConnectionCheckGeneration/F);B
A 0 0assert.match(saved,C 0/FcurrentSettings\C.marketBase 0!=D=D 0settingsToCheck\C.marketBase/F);B
A 0 0assert.match(draft,C 0/Fconst 0generation 0=D 0\C+B\C+BdraftConnectionCheckGeneration/F);B
A 0 0assert.match(draft,C 0/FconnectionDraftChanged\C(testedDraft,C 0currentDraft\C)/F);B
A 0 0assert.match(draft,C 0/Fè8¡1¨8å5D•5å5œC°0å5D€0æ6£3€0æ6ŸF¥5ç7»B“3æ6EœCïF¼Cˆ8å5°0šAæ6œCªAä4¿FDå5­D˜8ïF¼C‰9/F);B
A 0 0assert.doesNotMatch(draft,C 0/FsaveSettings\C(/F);B
A}D);B
Atest("2market 0connection 0changes 0invalidate 0all 0in-flight 0stock-research 0panels"2,C 0() 0=D>E 0{B
A 0 0assert.match(app,C 0/Fconst 0marketConnectionChanged 0=D 0previousSettings\C.apiKey 0!=D=D 0nextSettings\C.apiKey 0\C|C\C|C\Cs*previousSettings\C.marketBase 0!=D=D 0nextSettings\C.marketBase;B[B\Cs\CS]D*?Fif 0\C(marketConnectionChanged\C) 0\C{B[B\Cs\CS]D*?FrunSettingsApplyStep\C("2å5F–6æ6¶6ˆ8è8‚2¡1ç7¥5¨8æ60œCç7´4¢2è8¯F·7æ6±1‚2å5¤4±1è8´4¥5ïF¼CšA"2,C 0\C(\C) 0=D>E 0cancelStockSearchRequest\C(\C),C 0failures\C);B[B\Cs\CS]D*?FrunSettingsApplyStep\C("2å5¤4±1æ6•5ˆ8ä4¸8ªAè8‚2¡1ç7 0”4ç7©9¶6è8¯F·7æ6±1‚2å5¤4±1è8´4¥5ïF¼CšA"2,C 0\C(\C) 0=D>E\Cs*stockResearch\C.invalidate\C("2è8¡1ŒCæ6ƒ3…5è8¿FEæ6E¥5é9…5Dç7½D®Eå5·7²2å5F˜8æ6›B´4ïF¼CŒCæ6—7§7ç7 0”4ç7©9¶6è8¯F·7æ6±1‚2å5·7²2ä4½DœCå5ºAŸFïF¼CŒCè8¯F·7é9‡7Dæ6–6°0å5ŠA 0è8½D½Då5¯F¹9å5ºA”4è8µ5„4æ6–6™9ã3€0‚2"2\C),C 0failures\C);B/F);B
A 0 0assert.match(stockResearch,C 0/Ffunction 0invalidate\C(message=D"2è8¡1ŒCæ6ƒ3…5è8¿FEæ6E¥5é9…5Dç7½D®Eå5·7²2å5F˜8æ6›B´4ïF¼CŒCè8¯F·7é9‡7Dæ6–6°0å5ŠA 0è8½D½Dä4¸8ªAè8‚2¡1ç7 0”4ç7©9¶6è8µ5„4æ6–6™9ã3€0‚2"2\C)\C{B\Cs*reset\C(resolvedSymbol\C(\C)\C);B[B\Cs\CS]D*?Fif\C(!settings\C?F\C.apiKey\C)\C{BmarkApiKeyRequired\C(\C);Breturn;B\C}D/F);B
A 0 0assert.match(stockResearch,C 0/Ffunction 0reset\C(symbol\C)\C{B[B\Cs\CS]D*?FObject\C.keys\C(state\C.generation\C)\C.forEach\C(abort\C)/F);B
A}D);B
A
Atest("2service 0health 0and 0provider-list 0refreshes 0ignore 0superseded 0responses"2,C 0() 0=D>E 0{B
A 0 0assert.match(app,C 0/Flet 0serviceRefreshGeneration 0=D 00;B/F);B
A 0 0assert.match(app,C 0/Flet 0sourceRefreshGeneration 0=D 00;B/F);B
A 0 0assert.match(app,C 0/Fasync 0function 0refreshServices\C(\C) 0\C{B[B\Cs\CS]D*?Fconst 0generation 0=D 0\C+B\C+BserviceRefreshGeneration;B[B\Cs\CS]D*?Fif 0\C(generation 0!=D=D 0serviceRefreshGeneration\C) 0\C{B\Cs*return 0\C{B 0marketOK,C 0analysisOK,C 0marketState,C 0analysisState,C 0stale:A 0true 0\C}D;B\Cs*\C}D/F);B
A 0 0assert.match(app,C 0/Fasync 0function 0refreshSources\C(\C) 0\C{B[B\Cs\CS]D*?Fconst 0generation 0=D 0\C+B\C+BsourceRefreshGeneration;B[B\Cs\CS]D*?Fif 0\C(generation 0!=D=D 0sourceRefreshGeneration\C) 0return;B/F);B
A 0 0assert.match(app,C 0/Fif 0\C(generation 0!=D=D 0serviceRefreshGeneration\C) 0\C{B\Cs*return 0\C{B 0marketOK,C 0analysisOK,C 0marketState,C 0analysisState,C 0stale:A 0true 0\C}D;B\Cs*\C}D[B\Cs\CS]D*?FsetText\C("2#3connection-label"2,C 0connectionLabel\C)/F);B
A}D);B
A
Atest("2provider 0capability 0refresh 0clears 0old 0connection 0data 0and 0owns 0its 0loading 0state"2,C 0() 0=D>E 0{B
A 0 0const 0refreshSources 0=D 0app.match(/Fasync 0function 0refreshSources\C(\C) 0\C{B[B\Cs\CS]D*?F\C\Cn\C}D/F)?F.[B0]D 0|C|C 0"2"2;B
A 0 0assert.ok(refreshSources,C 0"2refreshSources 0implementation 0must 0exist"2);B
A 0 0assert.match(refreshSources,C 0/FshowMessage\C("2æ6­D£3å5œC¨8è8¯F»Bå5F–6å5½D“3å5‰9Dè8¿FEæ6E¥5é9…5Dç7½D®Eå5¯F¹9å5ºA”4ç7šA„4æ6•5°0æ6D®Eæ6ºA0è8ƒ3½Då5ŠA›Bâ2€0¦6"2\C);B[B\Cs\CS]D*?FsetSourceTextSafely\C("2#3source-count"2,C 0"2â2€0”4"2\C);B[B\Cs\CS]D*?Fstate\C.sourceCount 0=D 00/F);B
A 0 0assert.match(refreshSources,C 0/Fbutton\C.disabled 0=D 0true;B[B\Cs\CS]D*?Fbutton\C.setAttribute\C("2aria-busy"2,C 0"2true"2\C)/F);B
A 0 0assert.match(refreshSources,C 0/Fæ6­D£3å5œC¨8è8¯F»Bå5F–6å5½D“3å5‰9Dè8¿FEæ6E¥5é9…5Dç7½D®Eå5¯F¹9å5ºA”4ç7šA„4æ6•5°0æ6D®Eæ6ºA0è8ƒ3½Då5ŠA›B/F);B
A 0 0assert.match(refreshSources,C 0/FshowMessage\C(message,C 0"2empty-row 0error-text"2\C)/F);B
A 0 0assert.match(refreshSources,C 0/Ffinally 0\C{B[B\Cs\CS]D*?Fif 0\C(generation 0=D=D=D 0sourceRefreshGeneration\C) 0\C{B[B\Cs\CS]D*?Fif 0\C(!button\C) 0button 0=D 0\C$4\C("2#3reload-sources"2\C);B[B\Cs\CS]D*?Fif 0\C(button\C) 0\C{B[B\Cs\CS]D*?Fbutton\C.disabled 0=D 0false;B[B\Cs\CS]D*?Fbutton\C.removeAttribute\C("2aria-busy"2\C)/F);B
A}D);B
A
Atest("2filter 0changes 0clear 0stale 0errors 0even 0when 0a 0prior 0query 0returned 0no 0data"2,C 0() 0=D>E 0{B
A 0 0assert.match(app,C 0/Ffunction 0handleQuoteCriteriaChange\C([B^E)]D*\C) 0\C{B\Cs*cancelQuoteQuery\C(\C{B 0clearResults:A 0true,C[B\Cs\CS]D*?F\C}D\C);B\Cs*\C}D/F);B
A}D);B
A
Atest("2main 0quote 0query 0invalidates 0old 0snapshots 0when 0the 0symbol 0or 0filters 0change"2,C 0() 0=D>E 0{B
A 0 0assert.match(app,C 0/FquoteQueryGeneration:A 00/F);B
A 0 0assert.match(app,C 0/Ffunction 0cancelQuoteQuery\C(/F);B
A 0 0assert.match(app,C 0/Ffunction 0clearBarsDisplay\C(/F);B
A 0 0assert.match(app,C 0/FhandleSymbolInputChange\C(\C)/F);B
A 0 0assert.match(app,C 0/FhandleQuoteCriteriaChange\C(/F);B
A 0 0assert.match(app,C 0/Fcontroller\C.signal\C.aborted 0\C|C\C|C 0generation 0!=D=D 0state\C.quoteQueryGeneration/F);B
A 0 0assert.match(app,C 0/FgetDailyBars\C(state\C.settings,C 0symbol,C 0\C{B 0limit,C 0adjustment 0\C}D,C 0controller\C.signal\C)/F);B
A 0 0assert.match(app,C 0/FclearMarketQueryResults\C(symbol,C 0"2æ6œC¬Cæ6¬C¡1è8¡1ŒCæ6ƒ3…5ç7»B“3æ6EœCæ6—7 0æ6³3•5å5±1•5ç7¤4ºAïF¼CŒCæ6—7§7å5¿F«Bç7…5§7å5·7²2æ6¸8…5é9™9¤4ã3€0‚2"2\C)/F);B
A 0 0assert.match(app,C 0/Fè8¡1ŒCæ6ƒ3…5è8¿FEæ6E¥5é9…5Dç7½D®Eå5·7²2å5F˜8æ6›B´4ïF¼CŒCæ6—7§7å5¿F«Bç7…5§7å5·7²2æ6¸8…5é9™9¤4ïF¼CŒCè8¯F·7é9‡7Dæ6–6°0å5ŠA 0è8½D½Dã3€0‚2/F);B
A}D);B
A
Atest("2dynamic 0API 0and 0browser-local 0data 0are 0not 0rendered 0through 0HTML 0strings"2,C 0() 0=D>E 0{B
A 0 0assert.doesNotMatch(app,C 0/F\C.innerHTML\Cs*=D/F,C 0"2dynamic 0views 0should 0build 0DOM 0nodes 0and 0use 0textContent"2);B
A 0 0assert.match(app,C 0/Fdocument\C.createElement\C("2label"2\C)[B\Cs\CS]D*?Fsettings-nav-search/F);B
A 0 0assert.match(app,C 0/FtextContent 0=D 0"2æ60œCç7´4¢2è8®E¾Eç7½D®Eå5ˆ8†6ç7»B„4"2/F);B
A}D);B
Atest("2analysis 0task 0submission 0prevents 0concurrent 0duplicate 0requests 0and 0releases 0the 0guard"2,C 0() 0=D>E 0{B
A 0 0assert.match(app,C 0/FisCreatingTask:A 0false/F);B
A 0 0assert.match(app,C 0/Fasync 0function 0createTask\C(event\C)[B\Cs\CS]D*?Fevent\C.preventDefault\C(\C);B[B\Cs\CS]D*?Fif 0\C(state\C.isCreatingTask\C)[B\Cs\CS]D*?Få5ˆ8†6æ6E0ä4»B»Bå5ŠA¡1æ6­D£3å5œC¨8æ6F0ä4ºA¤4[B\Cs\CS]D*?Freturn;B[B\Cs\CS]D*?Fstate\C.isCreatingTask 0=D 0true;B[B\Cs\CS]D*?FparseAnalysisSymbols/F);B
A 0 0assert.match(app,C 0/Fcatch 0\C(error\C) 0\C{B\Cs*state\C.isCreatingTask 0=D 0false;B[B\Cs\CS]D*?Freturn;B/F);B
A 0 0assert.match(app,C 0/Ffinally 0\C{B[B\Cs\CS]D*?Fstate\C.isCreatingTask 0=D 0false;B[B\Cs\CS]D*?Fbutton\C.disabled 0=D 0false;B/F);B
A 0 0assert.match(apiClient,C 0/FheadersKey 0\C?F 0\C{B 0"2Idempotency-Key"2:A 0headersKey 0\C}D 0:A 0\C{B\C}D/F);B
A}D);B
A
Atest("2market 0API 0key 0help 0identifies 0who 0generates 0it 0and 0where 0it 0must 0be 0configured"2,C 0() 0=D>E 0{B
A 0 0assert.match(html,C 0/Fé9ƒ3¨8ç7½D²2ç7®E¡1ç70†6å5‘1˜8è8‡7ªAè8¡1ŒCç7”4ŸFæ6ˆ80å5¹9¶6é9…5Dç7½D®E/F);B
A 0 0assert.match(html,C 0/FMARKET_DATA_API_KEY/F);B
A 0 0assert.match(html,C 0/Fopenssl 0rand 0-hex 032/F);B
A 0 0assert.match(html,C 0/FX-API-Key/F);B
A 0 0assert.match(html,C 0/Fä4¸8Dæ6˜8¯Fç7¬C¬Cä4¸8‰9æ6–6¹9 0Provider 0å5¯F†6é9’2¥5/F);B
A}D);B
Atest("2market 0API 0key 0visibility 0is 0opt-in 0and 0resets 0when 0leaving 0settings"2,C 0() 0=D>E 0{B
A 0 0assert.match(html,C 0/Fid=D"2market-key-input"2 0type=D"2password"2/F);B
A 0 0assert.match(html,C 0/Fid=D"2toggle-market-key-visibility"2[B^E>E]D*aria-pressed=D"2false"2/F);B
A 0 0assert.match(app,C 0/Ffunction 0openSettings\C(\C)[B\Cs\CS]D*?FsetMarketKeyVisibility\C(false\C)/F);B
A 0 0assert.match(app,C 0/FpreviousView 0=D=D=D 0"2settings"2 0&6&6 0nextView 0!=D=D 0"2settings"2\C) 0setMarketKeyVisibility\C(false\C)/F);B
A}D);B
Atest("2global 0typography 0has 0a 0readable 0baseline 0and 0key 0UI 0text 0is 0not 0undersized"2,C 0() 0=D>E 0{B
A 0 0assert.match(html,C 0/Fid=D"2storage-boundary-title"2/F);B
A 0 0assert.match(stylesheet,C 0/F\C.settings-section-heading 0h2/F);B
A 0 0assert.match(stylesheet,C 0/Ffont-size:A\Cs*12px/F);B
A 0 0assert.match(stylesheet,C 0/Fline-height:A\Cs*1\C.[B456]D/F);B
A}D);B
Atest("2recent 0analysis 0history 0supports 0search,C 0single 0removal 0and 0local-only 0clearing"2,C 0() 0=D>E 0{B
A 0 0for 0(const 0id 0of 0[B"2recent-analysis-search"2,C 0"2clear-recent-analysis"2,C 0"2recent-analysis-feedback"2,C 0"2recent-analysis-list"2]D) 0{B
A 0 0 0 0assert.match(html,C 0new 0RegExp('id=D"2' 0+B 0id 0+B 0'"2'),C 0"2recent-history 0control 0must 0exist:A 0"2 0+B 0id);B
A 0 0}D
A 0 0assert.match(app,C 0/Ffunction 0renderRecentTasks\C([B\Cs\CS]D*?Frecent-analysis-search[B\Cs\CS]D*?Frecord\C.task_id[B\Cs\CS]D*?FtaskStatusLabel\C(record\C.status\C)/F);B
A 0 0assert.match(app,C 0/Ffunction 0removeRecentTaskReference\C(taskId,C 0analysisBase 0=D 0"2"2\C)[B\Cs\CS]D*?FremoveRecentTask\C(taskId,C 0localStorage,C 0undefined,C 0analysisBase\C)[B\Cs\CS]D*?Fconst 0persistedState 0=D 0safeReadRecentTasksState\C(\C)[B\Cs\CS]D*?FtargetStillExists[B\Cs\CS]D*?Fæ6œCDå5ŠA¡1ç7«B¯Fä4»B»Bå5ŠA¡1å5’2ŒCæ6ŠA¥5å5‘1ŠAä4¸8Dä4¼CšAè8¢2«Bå5ˆ8 0é9™9¤4/F);B
A 0 0assert.match(app,C 0/Ffunction 0removeRecentTaskReference[B\Cs\CS]D*?Fæ6—7 0æ6³3•5ç7¡1®Eè8®E¤4æ6œC€0è8¿F‘1ä4»B»Bå5ŠA¡1å5¼C•5ç7”4¨8å5·7²2ç7§7»Bé9™9¤4/F);B
A 0 0assert.match(app,C 0/Ffunction 0clearRecentTaskHistory\C(\C)[B\Cs\CS]D*?FclearRecentTasks\C(localStorage\C)[B\Cs\CS]D*?Fconst 0clearedState 0=D 0safeReadRecentTasksState\C(\C)[B\Cs\CS]D*?Fæ6œCªAè8ƒ3½Dç7¡1®Eè8®E¤4æ6œC€0è8¿F‘1ä4»B»Bå5ŠA¡1å5E†6å5F²2å5·7²2æ6¸8…5ç7©9ºA[B\Cs\CS]D*?Fæ6œCDå5ŠA¡1ç7«B¯Fè8®E°0å5½D•5æ6œCªAå5ˆ8 0é9™9¤4/F);B
A 0 0assert.match(app,C 0/Fstate\C.suppressedHistoryTaskId 0=D=D=D 0state\C.taskId/F);B
A 0 0assert.match(app,C 0/Fif 0\C(state\C.taskId 0=D=D=D 0taskId 0&6&6 0\C(!analysisBase 0\C|C\C|C 0state\C.taskAnalysisBase 0=D=D=D 0analysisBase\C)\C) 0\C{B[B\Cs\CS]D*?Fstate\C.suppressedHistoryTaskId 0=D 0taskId/F);B
A 0 0assert.match(recentTaskModule,C 0/Fexport 0function 0removeRecentTask\C(/F);B
A 0 0assert.match(recentTaskModule,C 0/Fexport 0function 0clearRecentTasks\C(/F);B
A 0 0assert.match(stylesheet,C 0/F\C.recent-analysis-search 0input:Afocus/F);B
A 0 0assert.match(stylesheet,C 0/F\C.recent-analysis-remove:Ahover/F);B
A}D);B
A
A
Atest("2light/Fdark 0theme 0is 0accessible,C 0persisted 0locally,C 0and 0independent 0of 0backend 0APIs"2,C 0async 0() 0=D>E 0{B
A 0 0const 0module 0=D 0await 0readFile(new 0URL("2../Fsrc/Ftheme.js"2,C 0import.meta.url),C 0"2utf8"2);B
A 0 0assert.match(html,C 0/Fname=D"2color-scheme"2 0content=D"2light 0dark"2/F);B
A 0 0assert.match(html,C 0/Fid=D"2theme-toggle"2[B^E>E]D*aria-label=D"2å5ˆ8‡7æ6D¢2ä4¸8»Bé9¢2˜8"2[B^E>E]D*aria-pressed=D"2false"2/F);B
A 0 0assert.match(app,C 0/FsetAttribute\C("2aria-label"2,C 0persistenceWarning 0\C?F 0"2å5ˆ8‡7æ6D¢2ä4¸8»Bé9¢2˜8ïF¼C›Bå5½D“3å5‰9Dä4¸8»Bé9¢2˜8æ6œCªAè8ƒ3½Dä4¿FDå5­D˜8"2 0:A 0"2å5ˆ8‡7æ6D¢2ä4¸8»Bé9¢2˜8"2\C)/F);B
A 0 0assert.match(app,C 0/Fconst 0warning 0=D 0persistenceWarning 0\C?F 0"2ïF¼C›Bå5½D“3å5‰9Dé9¡1µ5é9D¢2å5·7²2å5ˆ8‡7æ6D¢2ïF¼CŒCä4½D†6æ6œCªAè8ƒ3½Dç7¡1®Eè8®E¤4ä4¸8»Bé9¢2˜8å51Få5¥5½Då5·7²2ä4¿FDå5­D˜8"2 0:A 0"2"2/F);B
A 0 0assert.match(html,C 0/Fid=D"2theme-toggle-icon"2/F);B
A 0 0assert.match(html,C 0/Fid=D"2theme-toggle-label"2/F);B
A 0 0assert.match(app,C 0/Fimport 0\C{B 0applyTheme,C 0persistTheme,C 0readTheme(?F:A,C 0THEME_STORAGE_KEY)?F 0\C}D 0from 0"2\C.\C/Ftheme\C.js"2/F);B
A 0 0assert.match(app,C 0/F\C$4\C("2#3theme-toggle"2\C)\C.addEventListener\C("2click"2/F);B
A 0 0assert.match(stylesheet,C 0/F:Aroot\C[Bdata-theme=D"2light"2\C]D\Cs*\C{B/F);B
A 0 0assert.match(stylesheet,C 0/F--bg:A\Cs*#3f3f6f9/F);B
A 0 0assert.match(stylesheet,C 0/F:Aroot\C[Bdata-theme=D"2light"2\C]D 0\C.settings-dialog/F);B
A 0 0assert.match(stylesheet,C 0/F:Aroot\C[Bdata-theme=D"2light"2\C]D 0\C.analysis-result-card/F);B
A 0 0assert.match(stylesheet,C 0/F:Aroot\C[Bdata-theme=D"2light"2\C]D 0\C.raw-result 0pre/F);B
A 0 0assert.match(module,C 0/Fstock-provider-webui\C.theme\C.v1/F);B
A 0 0assert.doesNotMatch(module,C 0/Ffetch\C(|C\C/Fapi\C/F/F,C 0"2theme 0preference 0must 0not 0call 0backend 0APIs"2);B
A}D);B
A
Atest("2mobile 0navigation 0drawer 0restores 0readable 0labels 0and 0can 0close 0via 0overlay 0or 0Escape"2,C 0() 0=D>E 0{B
A 0 0const 0drawer 0=D 0app.match(/Ffunction 0setMobileNavigationOpen\C(open,C 0\C{B 0restoreFocus 0=D 0false 0\C}D 0=D 0\C{B\C}D\C) 0\C{B[B\Cs\CS]D*?F\C\Cn\C}D/F)?F.[B0]D 0|C|C 0"2"2;B
A 0 0assert.match(drawer,C 0/Faria-expanded/F);B
A 0 0assert.match(drawer,C 0/FisOpen 0\C?F 0"2å5…5³3é9—7­Då5¯F¼Cè8ˆ8ªA"2 0:A 0"2æ6‰9“3å5¼C€0å5¯F¼Cè8ˆ8ªA"2/F);B
A 0 0assert.match(drawer,C 0/Fif 0\C(wasOpen 0&6&6 0!isOpen 0&6&6 0restoreFocus\C) 0\C$4\C("2#3mobile-menu-toggle"2\C)\C.focus\C(\C)/F);B
A 0 0assert.match(app,C 0/Fevent\C.key 0=D=D=D 0"2Escape"2[B\Cs\CS]D*?FsetMobileNavigationOpen\C(false,C 0\C{B 0restoreFocus:A 0true 0\C}D\C)/F);B
A 0 0assert.match(app,C 0/F#3mobile-nav-overlay[B\Cs\CS]D*?FsetMobileNavigationOpen\C(false,C 0\C{B 0restoreFocus:A 0true 0\C}D\C)/F);B
A}D);B
Atest("2global 0navigation 0exposes 0all 0planned 0pages 0with 0hash 0routing 0and 0accessible 0active 0state"2,C 0() 0=D>E 0{B
A 0 0const 0views 0=D 0[B"2market"2,C 0"2tasks"2,C 0"2notifications"2,C 0"2chat"2,C 0"2screening"2,C 0"2alerts"2,C 0"2settings"2]D;B
A 0 0for 0(const 0view 0of 0views) 0{B
A 0 0 0 0assert.match(html,C 0new 0RegExp('data-view=D"2' 0+B 0view 0+B 0'"2'),C 0"2navigation 0link 0missing:A 0"2 0+B 0view);B
A 0 0 0 0assert.match(html,C 0new 0RegExp('id=D"2' 0+B 0view 0+B 0'-view"2'),C 0"2page 0shell 0missing:A 0"2 0+B 0view);B
A 0 0}D
A 0 0assert.match(app,C 0/Fconst 0VIEW_META 0=D 0Object\C.freeze\C(/F);B
A 0 0assert.match(app,C 0/Fwindow\C.addEventListener\C("2hashchange"2/F);B
A 0 0assert.match(app,C 0/FsetAttribute\C("2aria-current"2,C 0"2page"2\C)/F);B
A 0 0assert.match(html,C 0/Fid=D"2topbar-notifications"2/F,C 0"2topbar 0shortcut 0must 0exist 0without 0a 0fabricated 0count"2);B
A 0 0assert.doesNotMatch(html,C 0/Fid=D"2notification-count"2/F,C 0"2no 0unread 0badge 0without 0a 0real 0notification 0API"2);B
A}D);B
A
Atest("2new 0feature 0pages 0display 0unavailable 0states 0rather 0than 0production 0mock 0records"2,C 0() 0=D>E 0{B
A 0 0assert.match(html,C 0/Fé9€0šAç7ŸF¥5 0API 0å5°0šAæ6œCªAæ6E¥5å5…5¥5/F);B
A 0 0assert.match(html,C 0/FAgent 0API 0å5¾E…5æ6E¥5å5…5¥5/F);B
A 0 0assert.match(html,C 0/Fé9€0‰9è8‚2¡1 0API 0å5°0šAæ6œCªAæ6E¥5å5…5¥5/F);B
A 0 0assert.match(html,C 0/Få50Eç7«B¯Få5¾E…5æ6E¥5å5…5¥5/F);B
A 0 0assert.match(html,C 0/Fdisabled 0aria-disabled=D"2true"2/F);B
A 0 0assert.match(html,C 0/Fä4¸8Dä4¼CšAå5œC¨8æ6µ5Fè8§7ˆ8å5™9¨8é9‡7ŒCå51‡7å5»BºAã3€01å5ˆ8 0é9™9¤4æ6ˆ8–6ä4¿FDå5­D˜8ä4¼CšAè8¯FD/F);B
A}D);B
Atest("2new 0page 0layouts 0have 0responsive 0grids 0and 0disabled-state 0affordances"2,C 0() 0=D>E 0{B
A 0 0assert.match(stylesheet,C 0/F\C.feature-card-grid\Cs*\C{B[B^E}D]D*grid-template-columns:A 0repeat\C(3,C 0minmax\C(0,C 01fr\C)\C)/F);B
A 0 0assert.match(stylesheet,C 0/F@0media 0\C(max-width:A 0800px\C)[B\Cs\CS]D*?F\C.settings-card-grid 0\C{B 0grid-template-columns:A 0minmax\C(0,C 01fr\C);B 0\C}D/F);B
A 0 0assert.match(stylesheet,C 0/F@0media 0\C(max-width:A 0600px\C)[B\Cs\CS]D*?F\C.feature-form-grid 0\C{B 0grid-template-columns:A 0minmax\C(0,C 01fr\C);B 0\C}D/F);B
A 0 0assert.match(stylesheet,C 0/Fbutton:Adisabled,C 0input:Adisabled,C 0select:Adisabled,C 0textarea:Adisabled/F);B
A}D);B
A
A
Atest("2task 0detail 0opens 0an 0accessible 0run-flow 0drawer 0with 0separate 0Task 0and 0Run 0states"2,C 0() 0=D>E 0{B
A 0 0for 0(const 0id 0of 0[B"2run-flow-dialog"2,C 0"2run-flow-title"2,C 0"2close-run-flow-dialog"2,C 0"2task-status-pill"2,C 0"2task-run-status-pill"2,C 0"2task-created"2,C 0"2task-started"2,C 0"2task-finished"2,C 0"2task-symbols-value"2]D) 0{B
A 0 0 0 0assert.match(html,C 0new 0RegExp('id=D"2' 0+B 0id 0+B 0'"2'),C 0"2run-flow 0field 0is 0missing:A 0"2 0+B 0id);B
A 0 0}D
A 0 0assert.match(html,C 0/F<Cdialog 0class=D"2run-flow-dialog"2 0id=D"2run-flow-dialog"2/F);B
A 0 0assert.match(app,C 0/Ffunction 0openRunFlowDrawer\C(\C)[B\Cs\CS]D*?Fdialog\C.show\C(\C)/F);B
A 0 0assert.match(app,C 0/Ffunction 0renderSelectedTaskEvent\C(item\C)[B\Cs\CS]D*?FdetailLayout\C?F\C.classList\C.remove\C("2has-selected-event"2\C)[B\Cs\CS]D*?FdetailLayout\C?F\C.classList\C.add\C("2has-selected-event"2\C)/F);B
A 0 0assert.match(stylesheet,C 0/F\C.run-flow-dialog 0#3task-detail:Anot\C(\C[Bhidden\C]D\C):Anot\C(\C.has-selected-event\C)\Cs*\C{B\Cs*grid-template-columns:A\Cs*minmax\C(0,C\Cs*1fr\C);B\Cs*\C}D/F);B
A 0 0assert.match(app,C 0/Ffunction 0activateTaskReference\C(record\C)[B\Cs\CS]D*?FopenRunFlowDrawer\C(\C)[B\Cs\CS]D*?FpollTaskSafely\C(state\C.pollGeneration\C)/F);B
A 0 0assert.match(app,C 0/Ffunction 0renderTask\C(task\C)[B\Cs\CS]D*?Ftask\C.started_at[B\Cs\CS]D*?Ftask\C.finished_at/F);B
A 0 0assert.match(app,C 0/Ffunction 0renderRunStatus\C(status 0=D 0"2"2\C)[B\Cs\CS]D*?FrunStatusLabel\C(normalized\C)/F);B
A 0 0assert.match(app,C 0/Ffunction 0renderTaskResult\C(result\C)[B\Cs\CS]D*?FrenderRunStatus\C(result\C.status 0\C|C\C|C 0"2"2\C)/F);B
A 0 0assert.match(app,C 0/Fclose-run-flow-dialog/F);B
A}D);B
A
Atest("2run-flow 0events 0expose 0only 0server-provided 0durations 0and 0diagnostics"2,C 0() 0=D>E 0{B
A 0 0assert.match(app,C 0/Fevent\C.duration_ms 0!=D=D 0undefined 0&6&6 0event\C.duration_ms 0!=D=D 0null/F);B
A 0 0assert.match(app,C 0/Fevent\C.error_code/F);B
A 0 0assert.match(app,C 0/Fparts\C.join\C("2 0Â2·7 0"2\C)/F);B
A 0 0assert.match(apiClient,C 0/Fafter_sequence/F);B
A 0 0assert.match(app,C 0/F\C.sort\C(\C(a,C 0b\C) 0=D>E 0a\C.sequence 0- 0b\C.sequence\C)/F);B
A 0 0assert.doesNotMatch(app,C 0/Fcompleted_stages\Cs*\C/F\Cs*total_stages|CprogressPercent|CfakeDAG/Fi);B
A}D);B
A
A
Atest("2run-flow 0keeps 0cached 0events 0explicit 0during 0read 0errors 0and 0exports 0diagnostic 0context"2,C 0() 0=D>E 0{B
A 0 0assert.match(html,C 0/Fid=D"2task-events-read-status"2[B^E>E]D*role=D"2status"2[B^E>E]D*aria-live=D"2polite"2/F);B
A 0 0assert.match(app,C 0/Fconst 0readStatus 0=D 0\C$4\C("2#3task-events-read-status"2\C)/F);B
A 0 0assert.match(app,C 0/Få5·7²2ä4¿FDç7•5™9å5½D“3å5‰9Då5F¯Fç7”4¨8ä4ºA‹Bä4»B¶6/F);B
A 0 0assert.match(app,C 0/FeventsCount\C.textContent 0=D 0state\C.taskEventsError[B\Cs\CS]D*?Fè8¯F»Bå5F–6å5¤4±1è8´4¥5/F);B
A 0 0assert.match(app,C 0/Ftask_error:A\Cs*\C$4\C("2#3task-error"2\C)\C.textContent\C.trim\C(\C) 0\C|C\C|C 0null/F);B
A 0 0assert.match(app,C 0/Ftask_poll_status:A\Cs*\C$4\C("2#3task-poll-label"2\C)\C.textContent 0\C|C\C|C 0null/F);B
A 0 0assert.match(app,C 0/Fevent_read_status:A\Cs*\C{B[B\Cs\CS]D*?Flast_error:A\Cs*state\C.taskEventsError 0\C|C\C|C 0null[B\Cs\CS]D*?Fknown_event_count:A\Cs*state\C.taskEvents\C.size/F);B
A 0 0assert.match(stylesheet,C 0/F\C.task-events-read-status/F);B
A}D);B
A
A
Atest("2market 0hot-rank 0actions 0accept 0the 0symbol 0or 0code 0field 0as 0the 0stock 0identifier"2,C 0() 0=D>E 0{B
A 0 0assert.match(marketOverview,C 0/Fconst 0stock=Del\C("2td"2\C),Cbutton=Del\C("2button"2,C"2market-hot-stock-link"2\C),Csymbol=DString\C(item\C.symbol\C|C\C|Citem\C.code\C|C\C|C"2"2\C)\C.trim\C(\C);B/F);B
A 0 0assert.match(marketOverview,C 0/Fbutton\C.dataset\C.hotSymbol=Dsymbol;Bbutton\C.dataset\C.hotName=DString\C(item\C.name\C|C\C|Csymbol\C|C\C|C"2"2\C);Bbutton\C.disabled=D!symbol/F);B
A}D);B
A
Atest("2dragon-tiger 0rows 0only 0render 0stock 0navigation 0when 0a 0usable 0code 0exists"2,C 0() 0=D>E 0{B
A 0 0assert.match(marketActivity,C 0/Fconst 0symbol=DString\C(item\C.symbol\C|C\C|Citem\C.code\C|C\C|C"2"2\C)\C.trim\C(\C);B/F);B
A 0 0assert.match(marketActivity,C 0/Fif\C(symbol\C)\C{B\Cs*const 0button=Del\C("2button"2,C"2activity-stock-link"2,C"2æ6ŸF¥5ç7œC‹Bè8¡1ŒCæ6ƒ3…5"2\C);Bbutton\C.type=D"2button"2;Bbutton\C.dataset\C.dragonSymbol=Dsymbol/F);B
A 0 0assert.match(marketActivity,C 0/Fbutton\C[Bdata-dragon-symbol\C]D/F);B
A}D);B
A
Atest("2market 0activity 0clears 0stale 0calendar 0state 0for 0invalid 0input,C 0missing 0key,C 0loading,C 0and 0failure"2,C 0() 0=D>E 0{B
A 0 0assert.match(marketActivity,C 0/Ffunction 0clearReasons\C(id\C)/F);B
A 0 0assert.match(marketActivity,C 0/Fæ6—7¥5æ6œCŸFæ6—7 0æ6•5ˆ8[B\Cs\CS]D*?Fmeta\C("2trading-calendar-meta"2,C"2"2\C)[B\Cs\CS]D*?FclearReasons\C("2trading-calendar-reasons"2\C)/F);B
A 0 0assert.match(marketActivity,C 0/Fé9œC€0é9…5Dç7½D®E 0Key[B\Cs\CS]D*?Fæ6•5°0æ6D®Eæ6œCªAè8¯F»Bå5F–6 0Â2·7 0é9œC€0è8¦61 0API 0Key[B\Cs\CS]D*?FclearReasons\C("2trading-calendar-reasons"2\C)/F);B
A 0 0assert.match(marketActivity,C 0/Fbadge\C("2trading-calendar-quality"2,C"2å5ŠA 0è8½D½Dä4¸8­D"2,C"2unknown"2\C)/F);B
A 0 0assert.match(marketActivity,C 0/Fæ6­D£3å5œC¨8è8¯F»Bå5F–6ä4ºA¤4æ6˜8“3æ6—7¥5å5E†6â2€0¦6/F);B
A 0 0assert.match(marketActivity,C 0/Fæ6œC¬Cæ6¬C¡1æ6œCªAè8¿F”4å5›BEæ6•5°0æ6D®E[B\Cs\CS]D*?FclearReasons\C("2trading-calendar-reasons"2\C)/F);B
A 0 0assert.match(marketActivity,C 0/Fif\C(!append\C)[B\Cs\CS]D*?Få5ŠA 0è8½D½Dä4¸8­D[B\Cs\CS]D*?Fmeta\C("2dragon-tiger-meta"2,C"2"2\C)/F);B
A 0 0assert.match(marketActivity,C 0/Fé9¾E™9è8™9Eæ6¦6œCæ6šA‚2ä4¸8Då5F¯Fç7”4¨8/F);B
A 0 0assert.match(marketActivity,C 0/Fmeta\C("2dragon-tiger-meta"2,C"2"2\C)/F);B
A}D);B
A
A
Atest("2trading 0calendar 0aligns 0actual 0dates 0and 0keeps 0missing 0status 0distinct 0from 0market 0closure"2,C 0() 0=D>E 0{B
A 0 0assert.match(marketActivity,C 0/Ffunction 0calendarTimestamp\C(value\C)/F);B
A 0 0assert.match(marketActivity,C 0/Ffor\C(let 0timestamp=DstartTime;Btimestamp<C=DendTime;Btimestamp\C+B=D86400000\C)/F);B
A 0 0assert.match(marketActivity,C 0/Ffunction 0renderCalendarDay\C(root,Cdate,Citem\C)/F);B
A 0 0assert.match(marketActivity,C 0/Fitem\C.is_trading_day=D=D=Dtrue\C?F"2is-trading-day"2:Aitem\C.is_trading_day=D=D=Dfalse\C?F"2is-closed-day"2:A"2is-unknown-day"2/F);B
A 0 0assert.match(marketActivity,C 0/Fis-missing-day[B\Cs\CS]D*?Fæ6E¥5å5F£3æ6œCªAè8¿F”4å5›BEè8¯F¥5æ6—7¥5æ6œCŸFç7šA„4ä4ºA¤4æ6˜8“3æ6—7¥5ç7ŠA¶6æ6€01/F);B
A 0 0assert.match(marketActivity,C 0/FmissingDates>E0\C|C\C|CreportedMismatch\C|C\C|CrangeMismatch/F);B
A 0 0assert.match(stylesheet,C 0/F\C.trading-calendar-day\C.is-unknown-day 0span,C[B\Cs\CS]D*?F\C.trading-calendar-day\C.is-missing-day 0span/F);B
A}D);B
A
Atest("2market 0activity 0date 0filter 0changes 0cancel 0old 0requests 0and 0clear 0mismatched 0results"2,C 0() 0=D>E 0{B
A 0 0assert.match(marketActivity,C 0/Ffunction 0invalidateDragonFilter\C(\C)[B\Cs\CS]D*?Fabort\C("2dragon"2\C);B[B\Cs\CS]D*?FdragonHasMore=Dfalse[B\Cs\CS]D*?Fä4ºA¤4æ6˜8“3æ6—7¥5æ6œCŸFå5·7²2å5F˜8å5ŒC–6/F);B
A 0 0assert.match(marketActivity,C 0/Ffunction 0invalidateCalendarFilter\C(\C)[B\Cs\CS]D*?Fabort\C("2calendar"2\C);B[B\Cs\CS]D*?Fæ6—7¥5æ6œCŸFç7­D›Bé9€0‰9å5·7²2å5F˜8æ6›B´4[B\Cs\CS]D*?Fæ6—7¥5æ6œCŸFè8ŒCƒ3å5›B´4å5·7²2å5F˜8å5ŒC–6/F);B
A 0 0assert.match(marketActivity,C 0/F\C$4\C("2#3dragon-tiger-date"2\C)\C.addEventListener\C("2change"2,CinvalidateDragonFilter\C)/F);B
A 0 0assert.match(marketActivity,C 0/F\C$4\C("2#3calendar-start-date"2\C)\C.addEventListener\C("2change"2,CinvalidateCalendarFilter\C)/F);B
A 0 0assert.match(marketActivity,C 0/F\C$4\C("2#3calendar-end-date"2\C)\C.addEventListener\C("2change"2,CinvalidateCalendarFilter\C)/F);B
A}D);B
A
Atest("2changing 0fundamentals 0report 0period 0cancels 0old 0results 0until 0reloaded"2,C 0() 0=D>E 0{B
A 0 0assert.match(stockResearch,C 0/Ffunction 0invalidateFundamentalsPeriod\C(\C)[B\Cs\CS]D*?Fabort\C("2fundamentals"2\C)[B\Cs\CS]D*?FclearPanel\C("2fundamentals"2,C"2è8´4¢2å5ŠA¡1æ6ŠA¥5å5‘1ŠAæ6œCŸFå5·7²2å5F˜8æ6›B´4[B\Cs\CS]D*?FsetQuality\C("2fundamentals-quality"2,C"2unknown"2,C"2å5¾E…5é9‡7Dæ6–6°0å5ŠA 0è8½D½D"2\C)/F);B
A 0 0assert.match(stockResearch,C 0/F\C$4\C("2#3fundamentals-year"2\C)\C?F\C.addEventListener\C("2change"2,CinvalidateFundamentalsPeriod\C)/F);B
A 0 0assert.match(stockResearch,C 0/F\C$4\C("2#3fundamentals-quarter"2\C)\C?F\C.addEventListener\C("2change"2,CinvalidateFundamentalsPeriod\C)/F);B
A}D);B
A
Atest("2manual 0trading-calendar 0date 0ranges 0keep 0month 0navigation 0in 0sync"2,C 0() 0=D>E 0{B
A 0 0assert.ok(marketActivity.includes("2function 0syncCalendarMonthNavigation()"2));B
A 0 0assert.ok(marketActivity.includes("2monthOffset=DselectedYear*12+BselectedMonth-(Number(current.year)*12+BNumber(current.month)-1)"2));B
A 0 0assert.match(marketActivity,C 0/Ffunction 0invalidateCalendarFilter\C(\C)[B\Cs\CS]D*?FsyncCalendarMonthNavigation\C(\C)/F);B
A 0 0assert.match(marketActivity,C 0/Fmeta\C("2calendar-month-label"2,Clabel\C)/F);B
A}D);B
A
Atest("2provider 0capability 0definitions 0expose 0each 0backend 0capability 0once"2,C 0() 0=D>E 0{B
A 0 0const 0definitionBlock 0=D 0app.match(/Fconst 0capabilityDefinitions 0=D 0\C[B([B\Cs\CS]D*?F)
A 0 0\C]D;B/F);B
A 0 0assert.ok(definitionBlock,C 0"2provider 0capability 0definitions 0must 0remain 0explicit"2);B
A 0 0const 0keys 0=D 0[B...definitionBlock[B1]D.matchAll(/F\C[B"2([B^E"2]D+B)"2,C\Cs*"2[B^E"2]D+B"2\C]D/Fg)]D.map((match) 0=D>E 0match[B1]D);B
A 0 0assert.equal(new 0Set(keys).size,C 0keys.length,C 0"2one 0capability 0must 0not 0render 0multiple 0labels"2);B
A 0 0assert.equal(keys.filter((key) 0=D>E 0key 0=D=D=D 0"2chip_capable"2).length,C 01);B
A 0 0assert.equal(keys.filter((key) 0=D>E 0key 0=D=D=D 0"2capital_flow_capable"2).length,C 01);B
A}D);B
A
Atest("2run-flow 0event 0details 0share 0normalized 0status 0labels 0and 0tones 0with 0the 0timeline"2,C 0() 0=D>E 0{B
A 0 0assert.match(app,C 0/Ffunction 0taskEventStatusKey\C(status\C)/F);B
A 0 0assert.match(app,C 0/Ffunction 0taskEventStatusLabel\C(status,C 0statusKey 0=D 0taskEventStatusKey\C(status\C)\C)/F);B
A 0 0assert.match(app,C 0/Ffunction 0taskEventStatusTone\C(statusKey\C)/F);B
A 0 0assert.match(app,C 0/Fstatus\C.textContent 0=D 0statusLabel/F);B
A 0 0assert.match(app,C 0/F\C[B"2ä4ºA‹Bä4»B¶6ç7ŠA¶6æ6€01"2,C 0statusLabel\C]D/F);B
A 0 0assert.match(app,C 0/Fconst 0statusKey 0=D 0taskEventStatusKey\C(event\C.status\C)/F);B
A 0 0assert.match(stylesheet,C 0/F\C.availability-pill\C.is-error/F);B
A 0 0assert.match(stylesheet,C 0/F\C.availability-pill\C.is-warning/F);B
A}D);B
A
A
Atest("2settings 0center 0groups 0configuration 0by 0ownership 0and 0prevents 0fake 0server 0writes"2,C 0() 0=D>E 0{B
A 0 0const 0targets 0=D 0[B...html.matchAll(/Fdata-settings-target=D"2([B^E"2]D+B)"2/Fg)]D.map((match) 0=D>E 0match[B1]D);B
A 0 0assert.equal(targets.length,C 010);B
A 0 0for 0(const 0target 0of 0targets) 0assert.match(html,C 0new 0RegExp('id=D"2' 0+B 0target 0+B 0'"2'));B
A 0 0assert.match(html,C 0/Få5­D˜8å5œC¨8æ6œCªAä4¿FDå5­D˜8ç7šA„4è8¿FEæ6E¥5é9…5Dç7½D®Eæ6›B´4æ6”4¹9/F);B
A 0 0assert.match(html,C 0/Fæ6µ5Fè8§7ˆ8å5™9¨8æ6œC¬Cå5œC°0/F);B
A 0 0assert.match(html,C 0/Få5FªAè8¯F»B/F);B
A 0 0assert.match(html,C 0/Få5¾E…5æ6E¥5å5…5¥5/F);B
A 0 0assert.match(html,C 0/Få5½D“3å5‰9Dæ6µ5Fè8§7ˆ8å5™9¨8/F);B
A 0 0assert.doesNotMatch(html,C 0/Fprovider-secret-input|Csave-provider-secret/F);B
A 0 0assert.match(app,C 0/FsetMarketKeyVisibility\C(false\C)/F);B
A}D);B
Atest("2notification 0center 0exposes 0explicit 0five-state 0UI 0without 0fake 0unread 0counts 0or 0records"2,C 0() 0=D>E 0{B
A 0 0for 0(const 0state 0of 0[B"2loading"2,C 0"2empty"2,C 0"2unavailable"2,C 0"2error"2,C 0"2loaded"2]D) 0{B
A 0 0 0 0assert.match(html,C 0new 0RegExp('data-notification-state=D"2' 0+B 0state 0+B 0'"2'));B
A 0 0}D
A 0 0assert.match(html,C 0/Fid=D"2notification-category-filter"2[B^E>E]D*disabled/F);B
A 0 0assert.match(html,C 0/Fid=D"2notification-status-filter"2[B^E>E]D*disabled/F);B
A 0 0assert.match(html,C 0/Fid=D"2notification-search"2 0type=D"2search"2 0disabled/F);B
A 0 0assert.match(html,C 0/Fid=D"2refresh-notifications"2[B^E>E]D*disabled/F);B
A 0 0assert.match(html,C 0/Fid=D"2notification-detail-empty"2/F);B
A 0 0assert.match(html,C 0/Fid=D"2notification-detail-content"2[B^E>E]D*hidden/F);B
A 0 0assert.match(html,C 0/Fä4¸8Dä4¼CšAç7”4ŸFæ6ˆ80æ6¨8¡1æ6‹BŸFé9€0šAç7ŸF¥5ã3€01è8™9šAå51‡7æ6œCªAè8¯F»Bè8§7’2æ6 0‡7/F);B
A 0 0assert.match(html,C 0/Fä4¸8Dä4¼CšAå5FªAå5œC¨8æ6µ5Fè8§7ˆ8å5™9¨8é9‡7ŒCå51‡7ä4¿FDå5­D˜8/F);B
A 0 0assert.match(app,C 0/Ffunction 0renderNotificationState\C(snapshot\C)[B\Cs\CS]D*?FresolveNotificationViewState\C(snapshot\C)/F);B
A 0 0assert.match(app,C 0/FrenderNotificationState\C(\C{B 0supported:A 0false 0\C}D\C)/F);B
A 0 0assert.doesNotMatch(html,C 0/Fnotification-count|Cunread-count|Cæ6œCªAè8¯F»B\C\Cs*\C\Cd+B/Fi);B
A}D);B
A
A
Atest("2chat 0workbench 0has 0context 0isolation 0and 0no 0production 0mock 0replies"2,C 0() 0=D>E 0{B
A 0 0for 0(const 0id 0of 0[B
A 0 0 0 0"2chat-session-list-state"2,C 0"2chat-new-session"2,C 0"2chat-stock-context"2,C 0"2chat-clear-context"2,C
A 0 0 0 0"2chat-mode-select"2,C 0"2chat-message-list"2,C 0"2chat-composer-input"2,C 0"2chat-send"2,C 0"2chat-stop"2,C
A 0 0 0 0"2chat-copy-message"2,C 0"2chat-export-session"2,C 0"2chat-retry-message"2
A 0 0]D) 0assert.match(html,C 0new 0RegExp('id=D"2' 0+B 0id 0+B 0'"2'),C 0"2missing 0chat 0control:A 0"2 0+B 0id);B
A 0 0for 0(const 0state 0of 0[B"2loading"2,C 0"2empty"2,C 0"2unavailable"2,C 0"2error"2,C 0"2loaded"2]D) 0{B
A 0 0 0 0assert.match(html,C 0new 0RegExp('data-chat-state=D"2' 0+B 0state 0+B 0'"2'));B
A 0 0}D
A 0 0assert.match(html,C 0/Fä4¸8Dä4¼CšAç7”4¨8å5D•5æ6¬C¡1å5ˆ8†6æ6E0æ6ŠA¥5å5‘1ŠAå5†6’2å5…5…5å5¤4šAè8½D®E 0AI 0å5¯F¹9è8¯FD/F);B
A 0 0assert.match(html,C 0/Fä4¸8Dä4¼CšAè8‡7ªAå5ŠA¨8å5¤4Dç7”4¨8å5E†6å5F²2ä4¸8ŠAä4¸8‹Bæ6–6‡7/F);B
A 0 0assert.match(app,C 0/Ffunction 0renderChatState\C(snapshot\C)[B\Cs\CS]D*?FresolveChatViewState\C(snapshot\C)/F);B
A 0 0assert.match(app,C 0/FchatStockContextCleared:A 0false/F);B
A 0 0const 0syncStart 0=D 0app.indexOf("2function 0syncChatStockContextFromMarket() 0{B"2);B
A 0 0const 0syncEnd 0=D 0app.indexOf(String.fromCharCode(10) 0+B 0"2function 0"2,C 0syncStart);B
A 0 0const 0contextSync 0=D 0app.slice(syncStart,C 0syncEnd);B
A 0 0assert.match(contextSync,C 0/Fif 0\C(state\C.chatStockContextCleared\C) 0\C{B[B\Cs\CS]D*?Fstate\C.chatStockContext 0=D 0null;B[B\Cs\CS]D*?Freturn;B/F);B
A 0 0assert.match(contextSync,C 0/Fstate\C.quote 0&6&6 0typeof 0state\C.quote\C.symbol/F);B
A 0 0assert.match(app,C 0/Fif 0\C(nextView 0=D=D=D 0"2chat"2 0&6&6 0previousView 0!=D=D 0"2chat"2\C) 0syncChatStockContextFromMarket\C(\C)/F);B
A 0 0assert.match(app,C 0/Fstate\C.chatStockContextCleared 0=D 0true;B\Cs*state\C.chatStockContext 0=D 0null;B[B\Cs\CS]D*?FrenderChatStockContext\C(\C)/F);B
A 0 0const 0quoteStart 0=D 0app.indexOf("2async 0function 0runQuoteQuery(event) 0{B"2);B
A 0 0const 0quoteEnd 0=D 0app.indexOf(String.fromCharCode(10) 0+B 0"2function 0"2,C 0quoteStart);B
A 0 0const 0quoteQuery 0=D 0app.slice(quoteStart,C 0quoteEnd);B
A 0 0assert.match(quoteQuery,C 0/Fconst 0generation 0=D 0\C+B\C+Bstate\C.quoteQueryGeneration;B[B\Cs\CS]D*?Ftry 0\C{B\Cs*state\C.chatStockContextCleared 0=D 0false;B/F);B
A 0 0assert.match(app,C 0/FrenderChatState\C(\C{B 0supported:A 0false 0\C}D\C)/F);B
A 0 0assert.match(html,C 0/Fid=D"2chat-send"2 0type=D"2button"2 0disabled/F);B
A 0 0assert.match(html,C 0/Fid=D"2chat-stop"2 0type=D"2button"2 0disabled/F);B
A 0 0assert.match(html,C 0/Fid=D"2chat-mode-select"2 0disabled/F);B
A 0 0assert.doesNotMatch(app,C 0/FCHAT_SESSION_KEY|ClocalStorage.*chat/Fi);B
A}D);B
A
A
Atest("2screening 0workbench 0distinguishes 0unavailable 0capability 0from 0disabled 0and 0empty 0results"2,C 0() 0=D>E 0{B
A 0 0for 0(const 0id 0of 0[B
A 0 0 0 0"2screening-capability-pill"2,C 0"2screening-summary-capability"2,C 0"2screening-summary-run"2,C
A 0 0 0 0"2screening-summary-candidates"2,C 0"2screening-summary-last-run"2,C 0"2screening-market"2,C
A 0 0 0 0"2screening-universe"2,C 0"2screening-strategy"2,C 0"2screening-candidate-limit"2,C 0"2screening-min-change"2,C
A 0 0 0 0"2screening-min-turnover"2,C 0"2screening-submit"2,C 0"2screening-refresh-run"2,C 0"2screening-candidate-search"2,C
A 0 0 0 0"2screening-quality-filter"2,C 0"2screening-candidate-list"2,C 0"2screening-history-list"2,C
A 0 0 0 0"2screening-history-prev"2,C 0"2screening-history-next"2
A 0 0]D) 0assert.match(html,C 0new 0RegExp('id=D"2' 0+B 0id 0+B 0'"2'),C 0"2missing 0screening 0element:A 0"2 0+B 0id);B
A 0 0for 0(const 0state 0of 0[B"2loading"2,C 0"2idle"2,C 0"2queued"2,C 0"2running"2,C 0"2completed"2,C 0"2partial"2,C 0"2failed"2,C 0"2unknown"2,C 0"2error"2,C 0"2unavailable"2]D) 0{B
A 0 0 0 0assert.match(html,C 0new 0RegExp('data-screening-run-state=D"2' 0+B 0state 0+B 0'"2'));B
A 0 0}D
A 0 0for 0(const 0state 0of 0[B"2loading"2,C 0"2empty"2,C 0"2error"2,C 0"2unavailable"2,C 0"2loaded"2]D) 0{B
A 0 0 0 0assert.match(html,C 0new 0RegExp('data-screening-results-state=D"2' 0+B 0state 0+B 0'"2'));B
A 0 0 0 0assert.match(html,C 0new 0RegExp('data-screening-history-state=D"2' 0+B 0state 0+B 0'"2'));B
A 0 0}D
A 0 0const 0start 0=D 0html.indexOf('id=D"2screening-view"2');B
A 0 0const 0end 0=D 0html.indexOf('id=D"2alerts-view"2',C 0start);B
A 0 0const 0screeningView 0=D 0html.slice(start,C 0end);B
A 0 0assert.doesNotMatch(screeningView,C 0/F600519|Cè8´4µ5å5·7Eè8ŒC…5å5F°0|Cç7¤4ºAä4¾E‹Bå5€0™9é9€0‰9|Cæ6¨8¡1æ6‹BŸFå5ˆ8†6æ6•5°0/F);B
A 0 0assert.match(app,C 0/Ffunction 0renderScreeningCapability\C(snapshot\C)[B\Cs\CS]D*?FresolveScreeningCapability\C(snapshot\C)/F);B
A 0 0assert.match(app,C 0/Ffunction 0renderScreeningTask\C(snapshot\C)[B\Cs\CS]D*?FresolveScreeningTaskState\C(snapshot\C)/F);B
A 0 0assert.match(app,C 0/Ffunction 0renderScreeningCandidates\C(snapshot\C)[B\Cs\CS]D*?FresolveScreeningCollectionState\C(snapshot,C 0"2candidates"2\C)/F);B
A 0 0assert.match(app,C 0/Ffunction 0renderScreeningHistory\C(snapshot\C)[B\Cs\CS]D*?FresolveScreeningCollectionState\C(snapshot,C 0"2items"2\C)/F);B
A 0 0assert.match(app,C 0/FrenderScreeningCapability\C(\C{B 0supported:A 0false 0\C}D\C)/F);B
A 0 0assert.match(app,C 0/FrenderScreeningTask\C(\C{B 0supported:A 0false 0\C}D\C)/F);B
A 0 0assert.match(app,C 0/FrenderScreeningCandidates\C(\C{B 0supported:A 0false 0\C}D\C)/F);B
A 0 0assert.match(app,C 0/FrenderScreeningHistory\C(\C{B 0supported:A 0false 0\C}D\C)/F);B
A 0 0assert.match(screeningView,C 0/Fid=D"2screening-submit"2 0type=D"2button"2 0disabled/F);B
A 0 0assert.doesNotMatch(app,C 0/Fscreening.*localStorage/Fi);B
A}D);B
A
A
Atest("2alert 0center 0separates 0rule 0CRUD,C 0evaluation 0state,C 0trigger 0history 0and 0delivery 0attempts"2,C 0() 0=D>E 0{B
A 0 0for 0(const 0id 0of 0[B
A 0 0 0 0"2alert-capability-pill"2,C 0"2alert-rules-status-pill"2,C 0"2alert-rule-name"2,C 0"2alert-rule-scope"2,C 0"2alert-rule-type"2,C
A 0 0 0 0"2alert-rule-threshold"2,C 0"2alert-rule-severity"2,C 0"2alert-rule-enabled"2,C 0"2alert-rule-submit"2,C
A 0 0 0 0"2alert-rule-test"2,C 0"2alert-evaluation-status-pill"2,C 0"2alert-history-symbol"2,C 0"2alert-history-rule"2,C
A 0 0 0 0"2alert-history-status"2,C 0"2alert-history-list"2,C 0"2alert-delivery-status-pill"2,C 0"2alert-delivery-list"2
A 0 0]D) 0assert.match(html,C 0new 0RegExp('id=D"2' 0+B 0id 0+B 0'"2'),C 0"2missing 0alert 0control:A 0"2 0+B 0id);B
A 0 0for 0(const 0state 0of 0[B"2loading"2,C 0"2empty"2,C 0"2error"2,C 0"2unavailable"2,C 0"2loaded"2]D) 0{B
A 0 0 0 0assert.match(html,C 0new 0RegExp('data-alert-rules-state=D"2' 0+B 0state 0+B 0'"2'));B
A 0 0 0 0assert.match(html,C 0new 0RegExp('data-alert-history-state=D"2' 0+B 0state 0+B 0'"2'));B
A 0 0 0 0assert.match(html,C 0new 0RegExp('data-alert-delivery-state=D"2' 0+B 0state 0+B 0'"2'));B
A 0 0}D
A 0 0for 0(const 0state 0of 0[B"2loading"2,C 0"2idle"2,C 0"2triggered"2,C 0"2not-triggered"2,C 0"2evaluation-error"2,C 0"2degraded"2,C 0"2skipped"2,C 0"2error"2,C 0"2unknown"2,C 0"2unavailable"2]D) 0{B
A 0 0 0 0assert.match(html,C 0new 0RegExp('data-alert-evaluation-state=D"2' 0+B 0state 0+B 0'"2'));B
A 0 0}D
A 0 0const 0start 0=D 0html.indexOf('id=D"2alerts-view"2');B
A 0 0const 0end 0=D 0html.indexOf('id=D"2settings-view"2',C 0start);B
A 0 0const 0alertsView 0=D 0html.slice(start,C 0end);B
A 0 0assert.match(alertsView,C 0/Fä4¸8Dä4¼CšAå51‡7ä4¿FDå5­D˜8è8§7„4å5ˆ8™9/F);B
A 0 0assert.match(alertsView,C 0/Fä4¸8Dè8ƒ3½Dé9D™9é9»B˜8å5½D“3ä4½DœCæ6œCªAè8§7¦6å5F‘1/F);B
A 0 0assert.match(alertsView,C 0/Fä4¸8Dä4¼CšAå5±1•5ç7¤4ºAæ6¨8¡1æ6‹BŸFç7šA„4â2€0œCå5F‘1é9€01æ6ˆ80å5ŠAŸFâ2€0D/F);B
A 0 0assert.doesNotMatch(alertsView,C 0/F600519|Cè8´4µ5å5·7Eè8ŒC…5å5F°0|Cç7¤4ºAä4¾E‹Bè8§7„4å5ˆ8™9|Cç7¤4ºAä4¾E‹Bè8§7¦6å5F‘1/F);B
A 0 0assert.match(app,C 0/Ffunction 0renderAlertCapability\C(snapshot\C)[B\Cs\CS]D*?FresolveAlertCapability\C(snapshot\C)/F);B
A 0 0assert.match(app,C 0/Ffunction 0renderAlertEvaluation\C(snapshot\C)[B\Cs\CS]D*?FresolveAlertEvaluationState\C(snapshot\C)/F);B
A 0 0assert.match(app,C 0/FrenderAlertCapability\C(\C{B 0supported:A 0false 0\C}D\C)/F);B
A 0 0assert.match(app,C 0/FrenderAlertEvaluation\C(\C{B 0supported:A 0false 0\C}D\C)/F);B
A 0 0assert.match(html,C 0/Fid=D"2alert-rule-submit"2 0type=D"2submit"2/F);B
A 0 0assert.match(html,C 0/Fid=D"2alert-rule-test"2 0type=D"2button"2 0disabled/F);B
A 0 0assert.doesNotMatch(app,C 0/Falert.*localStorage/Fi);B
A}D);B
A
A
Atest("2stock-name 0autocomplete 0uses 0the 0real 0market 0API 0and 0accessible,C 0stale-safe 0controls"2,C 0() 0=D>E 0{B
A 0 0assert.match(html,C 0/Faria-autocomplete=D"2list"2/F);B
A 0 0assert.match(html,C 0/Fid=D"2stock-search-results"2[B^E>E]D*role=D"2listbox"2/F);B
A 0 0assert.match(html,C 0/Fid=D"2stock-search-status"2[B^E>E]D*role=D"2status"2/F);B
A 0 0assert.match(apiClient,C 0/Fexport 0function 0searchStocks\C(/F);B
A 0 0assert.match(app,C 0/Ffunction 0scheduleStockSearch\C(\C)/F);B
A 0 0assert.match(app,C 0/Fnew 0AbortController\C(\C)/F);B
A 0 0assert.match(app,C 0/Fgeneration 0!=D=D 0state\C.stockSearchGeneration/F);B
A 0 0assert.match(app,C 0/FaddEventListener\C("2compositionstart"2/F);B
A 0 0assert.match(app,C 0/FaddEventListener\C("2compositionend"2/F);B
A 0 0assert.match(app,C 0/Ffunction 0chooseStockSearchResult\C(/F);B
A 0 0assert.match(app,C 0/Ferror\C.status 0=D=D=D 0404/F);B
A 0 0assert.match(stylesheet,C 0/F\C.stock-search-option/F);B
A 0 0assert.doesNotMatch(app,C 0/FlocalStorage[B^E
A]D*stockSearch/Fi,C 0"2stock 0results 0must 0come 0from 0the 0API,C 0not 0browser-side 0mock 0data"2);B
A}D);B
A
A
Atest("2Run 0Flow 0summary 0and 0event 0diagnostics 0remain 0grounded 0in 0persisted 0Task,C 0StageEvent 0and 0RunResult 0data"2,C 0() 0=D>E 0{B
A 0 0for 0(const 0id 0of 0[B
A 0 0 0 0"2task-summary-event-count"2,C
A 0 0 0 0"2task-summary-latest-event"2,C
A 0 0 0 0"2task-summary-quality"2,C
A 0 0 0 0"2task-summary-warning-count"2,C
A 0 0 0 0"2task-event-detail"2,C
A 0 0 0 0"2task-event-detail-fields"2,C
A 0 0 0 0"2task-event-json"2
A 0 0]D) 0{B
A 0 0 0 0assert.match(html,C 0new 0RegExp('id=D"2' 0+B 0id 0+B 0'"2'),C 0"2Run 0Flow 0must 0include 0"2 0+B 0id);B
A 0 0}D
A 0 0assert.match(app,C 0/Ffunction 0renderTaskFlowSummary\C(\C)/F);B
A 0 0assert.match(app,C 0/Ffunction 0renderSelectedTaskEvent\C(item\C)/F);B
A 0 0assert.match(app,C 0/Fbutton\C[Bdata-task-event-sequence\C]D/F);B
A 0 0assert.match(app,C 0/Fstate\C.taskEventsLoaded/F);B
A 0 0assert.match(app,C 0/Fevent\C.duration_ms/F);B
A 0 0assert.match(app,C 0/Fevent\C.error_code/F);B
A 0 0assert.match(app,C 0/Fresult\C?F\C.quality\C?F\C.core_pass/F);B
A 0 0assert.match(app,C 0/FArray\C.isArray\C(warnings\C)/F);B
A 0 0assert.match(stylesheet,C 0/F\C.run-flow-summary/F);B
A 0 0assert.match(stylesheet,C 0/F\C.event-row\C[Baria-pressed=D"2true"2\C]D/F);B
A 0 0assert.doesNotMatch(app,C 0/Fcompleted_stages\Cs*\C/F\Cs*\Cw+B|CprogressPercent|C100\Cs*\C*\Cs*.*completed_stages/Fi,C
A 0 0 0 0"2Run 0Flow 0must 0not 0infer 0or 0fabricate 0progress 0percentages"2);B
A}D);B
A
A
Atest("2stock 0research 0toolbar 0controls 0use 0readable 0labels 0and 0touch-sized 0targets"2,C 0() 0=D>E 0{B
A 0 0assert.match(finalCssRuleBody("2.stock-research-toolbar 0label"2),C 0/Ffont-size:A12px/F);B
A 0 0assert.match(finalCssRuleBody("2.stock-research-toolbar 0select"2),C 0/Fmin-height:A38px/F);B
A 0 0assert.match(finalCssRuleBody("2.stock-research-toolbar 0select"2),C 0/Ffont-size:A12px/F);B
A 0 0const 0toolbarButtonRules 0=D 0[B...stylesheet.matchAll(/F\C.stock-research-toolbar 0\C.button\Cs*\C{B([B^E}D]D*)\C}D/Fg)]D.map((match) 0=D>E 0match[B1]D);B
A 0 0assert.ok(toolbarButtonRules.some((body) 0=D>E 0/Fmin-height:A\Cs*38px/F.test(body)),C 0"2toolbar 0button 0must 0have 0a 0touch-sized 0minimum 0height"2);B
A 0 0assert.ok(toolbarButtonRules.some((body) 0=D>E 0/Ffont-size:A\Cs*12px/F.test(body)),C 0"2toolbar 0button 0label 0must 0remain 0readable"2);B
A}D);B
A
Atest("2market 0overview 0and 0activity 0use 0readable 0secondary 0text 0and 0touch-sized 0controls"2,C 0() 0=D>E 0{B
A 0 0assert.match(finalCssRuleBody("2.market-overview-updated"2),C 0/Ffont-size:A 012px/F);B
A 0 0assert.match(finalCssRuleBody("2.market-overview-subheading 0p"2),C 0/Ffont-size:A 012px/F);B
A 0 0assert.match(finalCssRuleBody("2.market-index-identity 0small"2),C 0/Ffont-size:A 012px/F);B
A 0 0assert.match(finalCssRuleBody("2.market-index-changes 0span"2),C 0/Ffont-size:A 012px/F);B
A 0 0assert.match(finalCssRuleBody("2.market-widget-meta"2),C 0/Ffont-size:A 012px/F);B
A 0 0assert.match(finalCssRuleBody("2.market-breadth-metric 0small"2),C 0/Ffont-size:A 012px/F);B
A 0 0assert.match(finalCssRuleBody("2.market-table-primary"2),C 0/Ffont-size:A 012px/F);B
A 0 0assert.match(finalCssRuleBody("2.market-table-secondary"2),C 0/Ffont-size:A 012px/F);B
A 0 0assert.match(finalCssRuleBody("2.market-touch-time"2),C 0/Ffont-size:A 012px/F);B
A 0 0assert.match(finalCssRuleBody("2.market-activity-heading 0p"2),C 0/Ffont-size:A 012px/F);B
A 0 0assert.match(finalCssRuleBody("2.market-activity-controls 0label"2),C 0/Ffont-size:A 012px/F);B
A 0 0assert.match(finalCssRuleBody("2.market-activity-controls 0input"2),C 0/Fmin-height:A 038px/F);B
A 0 0assert.match(finalCssRuleBody("2.market-activity-controls 0.button"2),C 0/Fmin-height:A 038px/F);B
A 0 0const 0readabilityStart 0=D 0stylesheet.indexOf("2/F* 02026-10-27 0readability 0pass:A 0market 0overview,C 0activity 0table,C 0and 0calendar. 0*/F"2);B
A 0 0const 0mobileOverrideStart 0=D 0stylesheet.indexOf("2@0media 0(max-width:A 0700px)"2,C 0readabilityStart);B
A 0 0assert.ok(readabilityStart 0>E=D 00 0&6&6 0mobileOverrideStart 0>E 0readabilityStart,C 0"2the 0general 0readability 0pass 0should 0precede 0responsive 0overrides"2);B
A 0 0const 0generalRules 0=D 0stylesheet.slice(readabilityStart,C 0mobileOverrideStart);B
A 0 0assert.match(generalRules,C 0/F\C.market-activity-controls 0label 0\C{B[B^E}D]D*font-size:A 012px;B/F,C 0"2desktop 0labels 0need 0a 0readable 0baseline"2);B
A 0 0assert.match(generalRules,C 0/F\C.market-activity-controls 0input 0\C{B 0min-height:A 038px;B 0\C}D/F,C 0"2desktop 0date 0inputs 0need 0a 038px 0target"2);B
A 0 0assert.match(generalRules,C 0/F\C.market-activity-controls 0\C.button 0\C{B 0min-height:A 038px;B 0font-size:A 012px;B 0\C}D/F,C 0"2desktop 0buttons 0need 0a 0readable 038px 0baseline"2);B
A 0 0assert.match(finalCssRuleBody("2.activity-stock-link"2),C 0/Fmin-height:A 038px/F);B
A 0 0assert.match(finalCssRuleBody("2.calendar-month-navigation 0.icon-button"2),C 0/Fmin-width:A 038px;B 0min-height:A 038px/F);B
A 0 0assert.match(finalCssRuleBody("2.trading-calendar-weekdays 0span"2),C 0/Ffont-size:A 011px/F);B
A 0 0assert.match(stylesheet,C 0/F\C.trading-calendar-day 0span 0\C{B 0font-size:A 011px;B 0line-height:A 01\C.25;B 0text-align:A 0center;B 0overflow-wrap:A 0anywhere;B 0\C}D/F);B
A 0 0assert.match(finalCssRuleBody("2.trading-calendar-day 0span"2),C 0/Ffont-size:A 010px/F);B
A 0 0assert.match(stylesheet,C 0/F@0media 0\C(max-width:A 0700px\C) 0\C{B[B\Cs\CS]D*?F\C.market-overview-actions 0\C.button 0\C{B 0width:A 0100%5;B 0min-height:A 040px;B 0\C}D/F);B
A 0 0assert.match(stylesheet,C 0/F@0media 0\C(max-width:A 0600px\C) 0\C{B[B\Cs\CS]D*?F\C.trading-calendar-day 0span 0\C{B 0font-size:A 010px;B 0\C}D/F);B
A 0 0assert.match(finalCssRuleBody("2.market-activity-table"2),C 0/F-webkit-overflow-scrolling:A 0touch/F);B
A}D);B
A
A
Atest("2primary 0navigation 0keeps 0market 0modules 0on 0the 0homepage 0without 0quick-locate 0UI"2,C 0() 0=D>E 0{B
A 0 0for 0(const 0view 0of 0[B"2market"2,C 0"2stocks"2,C 0"2watchlist"2]D) 0{B
A 0 0 0 0assert.match(html,C 0new 0RegExp('class=D"2nav-link"2[B^E>E]D*data-view=D"2' 0+B 0view 0+B 0'"2'));B
A 0 0}D
A 0 0assert.match(html,C 0/Fid=D"2market-view"2/F);B
A 0 0assert.match(html,C 0/Fid=D"2stocks-view"2/F);B
A 0 0assert.match(html,C 0/Fid=D"2watchlist-view"2/F);B
A 0 0assert.doesNotMatch(html,C 0/Fid=D"2quick-locate"2|Cclass=D"2quick-locate"2|Cdata-view=D"2quick-locate"2/F);B
A 0 0assert.match(html,C 0/Fmarket-sector-body/F);B
A}D);B
Atest("2collapsed 0sidebar 0navigation 0remains 0accessible 0and 0unavailable 0modules 0disclose 0their 0status"2,C 0() 0=D>E 0{B
A 0 0assert.match(html,C 0/Fclass=D"2nav-link"2[B^E>E]D*data-view=D"2screening"2[B^E>E]D*aria-label=D"2é9€0‰9è8‚2¡1ä4¸8Eç7­D›Bé9€0‰9"2/F);B
A 0 0assert.match(html,C 0/Fclass=D"2nav-link"2[B^E>E]D*data-view=D"2alerts"2[B^E>E]D*aria-label=D"2å5‘1ŠAè8­D¦6"2/F);B
A 0 0assert.match(html,C 0/Fclass=D"2nav-link-state"2 0title=D"2å50Eç7«B¯Fæ6E¥5å5F£3å5¾E…5æ6E¥5å5…5¥5"2>Eå5¾E…5æ6E¥5å5…5¥5/F);B
A 0 0assert.match(stylesheet,C 0/Fnav-link-state/F);B
A}D);B
Atest("2æ6·7±1æ6µ5…5ä4¸8»Bé9¢2˜8ç7šA„4è8¾E…5å5ŠA©9æ6–6‡7å5­D—7å5œC¨8å5¸8¸8ç7”4¨8è8ƒ3ŒCæ6™9¯Fä4¸8ŠAè8¾E¾Eå5ˆ8°0 0WCAG 0AA 0å5¯F¹9æ6¯F”4å5ºA¦6"2,C 0() 0=D>E 0{B
A 0 0const 0parseHex 0=D 0(value) 0=D>E 0{B
A 0 0 0 0const 0hex 0=D 0value.trim().replace(/F^E#3/F,C 0"2"2);B
A 0 0 0 0const 0full 0=D 0hex.length 0=D=D=D 03 0?F 0hex.split("2"2).map((part) 0=D>E 0part 0+B 0part).join("2"2) 0:A 0hex;B
A 0 0 0 0const 0number 0=D 0Number.parseInt(full,C 016);B
A 0 0 0 0return 0[B(number 0>E>E 016) 0&6 0255,C 0(number 0>E>E 08) 0&6 0255,C 0number 0&6 0255]D.map((channel) 0=D>E 0{B
A 0 0 0 0 0 0const 0normalized 0=D 0channel 0/F 0255;B
A 0 0 0 0 0 0return 0normalized 0<C=D 00.04045 0?F 0normalized 0/F 012.92 0:A 0((normalized 0+B 00.055) 0/F 01.055) 0** 02.4;B
A 0 0 0 0}D);B
A 0 0}D;B
A 0 0const 0luminance 0=D 0(value) 0=D>E 0{B
A 0 0 0 0const 0[Br,C 0g,C 0b]D 0=D 0parseHex(value);B
A 0 0 0 0return 00.2126 0* 0r 0+B 00.7152 0* 0g 0+B 00.0722 0* 0b;B
A 0 0}D;B
A 0 0const 0contrast 0=D 0(foreground,C 0background) 0=D>E 0{B
A 0 0 0 0const 0values 0=D 0[Bluminance(foreground),C 0luminance(background)]D.sort((a,C 0b) 0=D>E 0b 0- 0a);B
A 0 0 0 0return 0(values[B0]D 0+B 00.05) 0/F 0(values[B1]D 0+B 00.05);B
A 0 0}D;B
A 0 0assert.match(stylesheet,C 0/F--text-dim:A\Cs*#3[B0-9a-f]D{B3,C6}D/Fi);B
A 0 0assert.match(stylesheet,C 0/F--panel:A\Cs*#3[B0-9a-f]D{B3,C6}D/Fi);B
A 0 0assert.match(html,C 0/Fid=D"2storage-boundary-title"2/F);B
A 0 0assert.ok(contrast("2#3666666"2,C 0"2#3ffffff"2) 0>E=D 05,C 0"2baseline 0secondary 0text 0contrast 0helper 0should 0use 0WCAG 0AA 0threshold"2);B
A}D);B
Atest("2è8¾E…5å5ŠA©9è8¯F´4æ6˜8Eã3€01ä4»B»Bå5ŠA¡1å5…5ƒ3ä4¿F¡1æ61¯Få5’2ŒCè8¾E“3å5…5¥5å5D 0ä4½DDæ6–6‡7å5­D—7ç7»BŸFä4¸8€0ä4½D¿Fç7”4¨8ä4¸8»Bé9¢2˜8æ6„4ŸFç7ŸF¥5ç7šA„4è8¾E…5å5ŠA©9æ6–6‡7å5­D—7 0token"2,C 0() 0=D>E 0{B
A 0 0for 0(const 0selector 0of 0[B
A 0 0 0 0"2.side-label"2,C
A 0 0 0 0"2.sidebar-footnote"2,C
A 0 0 0 0"2.task-facts 0small"2,C
A 0 0 0 0"2.event-row 0>E 0small"2,C
A 0 0 0 0"2.task-actions 0>E 0span"2
A 0 0]D) 0{B
A 0 0 0 0assert.match(
A 0 0 0 0 0 0baseCssDeclarationBlock(selector),C
A 0 0 0 0 0 0/Fcolor:A\Cs*var\C(--text-dim\C)\Cs*;B/F,C
A 0 0 0 0 0 0selector 0+B 0"2 0should 0inherit 0contrast-safe 0text 0color 0from 0the 0current 0theme"2
A 0 0 0 0);B
A 0 0}D
A
A 0 0assert.match(
A 0 0 0 0stylesheet,C
A 0 0 0 0/F\C.field 0input:A:Aplaceholder,C\Cs*\C.field 0textarea:A:Aplaceholder,C\Cs*\C.recent-analysis-search 0input:A:Aplaceholder,C\Cs*\C.watchlist-name-input:A:Aplaceholder\Cs*\C{B[B^E}D]D*color:A\Cs*var\C(--text-dim\C)/F,C
A 0 0 0 0"2form 0and 0local-search 0placeholders 0should 0use 0the 0shared 0contrast-safe 0token"2
A 0 0);B
A 0 0assert.match(
A 0 0 0 0stylesheet,C
A 0 0 0 0/F:Aroot\C[Bdata-theme=D"2light"2\C]D 0\C.field 0input:A:Aplaceholder,C[B\Cs\CS]D*?F\C.watchlist-name-input:A:Aplaceholder\Cs*\C{B\Cs*color:A\Cs*var\C(--text-dim\C);B/F,C
A 0 0 0 0"2light-theme 0placeholder 0overrides 0must 0not 0restore 0a 0low-contrast 0hard-coded 0color"2
A 0 0);B
A 0 0assert.match(stylesheet,C 0/F:Aroot\C[Bdata-theme=D"2light"2\C]D 0\C.side-label 0\C{B 0color:A 0var\C(--text-dim\C);B 0\C}D/F);B
A 0 0assert.match(stylesheet,C 0/F:Aroot\C[Bdata-theme=D"2light"2\C]D 0\C.sidebar-footnote 0\C{B 0color:A 0var\C(--text-dim\C);B 0\C}D/F);B
A 0 0assert.doesNotMatch(
A 0 0 0 0stylesheet,C
A 0 0 0 0/F\C.watchlist-name-input:A:Aplaceholder\Cs*\C{B\Cs*color:A\Cs*#3[B0-9a-fA-F]D{B3,C8}D\Cs*;B/F,C
A 0 0 0 0"2a 0later 0hard-coded 0watchlist 0placeholder 0rule 0must 0not 0override 0the 0shared 0theme 0token"2
A 0 0);B
A}D);B
A
A
A
Atest("2Run 0Flow 0ä4¸8Eç7»B“3æ6E„4å5ŒC–6ç7»B“3æ6EœCå5œC¨8æ6µ5…5è8‰9²2ä4¸8»Bé9¢2˜8ä4¸8‹Bä4½D¿Fç7”4¨8å5F¯Fè8¯F»Bç7šA„4å5‰9Dæ6™9¯Fæ6–6‡7å5­D—7"2,C 0() 0=D>E 0{B
A 0 0const 0mainTextSelectors 0=D 0[B
A 0 0 0 0"2.task-empty 0strong"2,C
A 0 0 0 0"2.task-id-row 0code"2,C
A 0 0 0 0"2.task-facts 0strong"2,C
A 0 0 0 0"2.event-body 0strong"2,C
A 0 0 0 0"2.boundary-grid 0strong"2,C
A 0 0 0 0"2.signal-result-card 0strong"2,C
A 0 0 0 0"2.analysis-result-block 0>E 0h3"2,C
A 0 0 0 0"2.analysis-result-card-heading 0strong"2,C
A 0 0 0 0"2.source-summary 0strong"2,C
A 0 0 0 0"2.supplemental-card-heading 0h3"2
A 0 0]D;B
A 0 0for 0(const 0selector 0of 0mainTextSelectors) 0{B
A 0 0 0 0assert.match(
A 0 0 0 0 0 0baseCssDeclarationBlock(selector),C
A 0 0 0 0 0 0/Fcolor:A\Cs*var\C(--text\C)\Cs*;B/F,C
A 0 0 0 0 0 0selector 0+B 0"2 0must 0use 0the 0theme-aware 0primary 0text 0token"2
A 0 0 0 0);B
A 0 0}D
A
A 0 0const 0dimTextSelectors 0=D 0[B
A 0 0 0 0"2.task-empty"2,C
A 0 0 0 0"2.task-id-row 0>E 0span"2,C
A 0 0 0 0"2.event-body 0p"2,C
A 0 0 0 0"2.boundary-grid 0p"2,C
A 0 0 0 0"2.task-result-note"2,C
A 0 0 0 0"2.task-result-metric 0small"2,C
A 0 0 0 0"2.signal-result-card 0p"2,C
A 0 0 0 0"2.analysis-result-block 0>E 0p"2,C
A 0 0 0 0"2.analysis-result-fields 0dt"2,C
A 0 0 0 0"2.analysis-result-empty"2,C
A 0 0 0 0"2.raw-result"2,C
A 0 0 0 0"2.empty-row"2,C
A 0 0 0 0"2.table-empty"2,C
A 0 0 0 0"2.query-hint"2,C
A 0 0 0 0"2.metric-foot"2,C
A 0 0 0 0"2.chart-empty"2,C
A 0 0 0 0"2.chart-foot"2,C
A 0 0 0 0"2.source-version"2,C
A 0 0 0 0"2.source-main 0small"2,C
A 0 0 0 0"2.table-footer"2,C
A 0 0 0 0"2.page-footer"2,C
A 0 0 0 0"2.recent-analysis-item-main 0small"2,C
A 0 0 0 0"2.watchlist-item-meta"2,C
A 0 0 0 0"2.supplemental-context"2,C
A 0 0 0 0"2.supplemental-card-heading 0small"2,C
A 0 0 0 0"2.supplemental-fields 0dt"2,C
A 0 0 0 0"2.supplemental-reasons"2,C
A 0 0 0 0"2.section-heading 0h2 0span"2,C
A 0 0 0 0"2.recent-analysis-feedback"2
A 0 0]D;B
A 0 0for 0(const 0selector 0of 0dimTextSelectors) 0{B
A 0 0 0 0assert.match(
A 0 0 0 0 0 0baseCssDeclarationBlock(selector),C
A 0 0 0 0 0 0/Fcolor:A\Cs*var\C(--text-dim\C)\Cs*;B/F,C
A 0 0 0 0 0 0selector 0+B 0"2 0must 0use 0the 0theme-aware 0secondary 0text 0token"2
A 0 0 0 0);B
A 0 0}D
A
A 0 0assert.match(baseCssDeclarationBlock("2.chart-axis-label"2),C 0/Ffill:A\Cs*var\C(--text-dim\C)\Cs*;B/F);B
A 0 0assert.match(stylesheet,C 0/F:Aroot\C[Bdata-theme=D"2light"2\C]D 0\C.analysis-result-badge\C.is-allowed 0\C{B 0color:A 0#308765e;B 0border-color:A 0#3b9e6d6;B 0\C}D/F);B
A 0 0assert.match(stylesheet,C 0/F:Aroot\C[Bdata-theme=D"2light"2\C]D 0\C.analysis-result-badge\C.is-blocked 0\C{B 0color:A 0#3b92f41;B 0border-color:A 0#3f2c3c9;B 0\C}D/F);B
A 0 0assert.match(stylesheet,C 0/F:Aroot\C[Bdata-theme=D"2light"2\C]D 0\C.task-result-warnings/F);B
A 0 0assert.match(stylesheet,C 0/F:Aroot\C[Bdata-theme=D"2light"2\C]D 0\C.inline-callout 0strong 0\C{B 0color:A 0#380540b;B 0\C}D/F);B
A 0 0assert.match(stylesheet,C 0/F:Aroot\C[Bdata-theme=D"2light"2\C]D 0\C.settings-feedback,C?F\Cs*[B\Cs\CS]D*?F:Aroot\C[Bdata-theme=D"2light"2\C]D 0\C.recent-analysis-feedback\C.is-error 0\C{B 0color:A 0#3b92f41;B 0\C}D/F);B
A 0 0assert.match(stylesheet,C 0/F:Aroot\C[Bdata-theme=D"2light"2\C]D 0\C.supplemental-feedback\C.is-warning 0\C{B 0color:A 0#380540b;B 0\C}D/F);B
A 0 0assert.match(stylesheet,C 0/F:Aroot\C[Bdata-theme=D"2light"2\C]D 0\C.supplemental-feedback\C.is-error 0\C{B 0color:A 0#3b92f41;B 0\C}D/F);B
A 0 0assert.match(stylesheet,C 0/F:Aroot\C[Bdata-theme=D"2light"2\C]D 0\C.soft-badge 0\C{B 0color:A 0#3245d95;B 0background:A 0#3eaf3ff;B 0\C}D/F);B
A 0 0assert.match(stylesheet,C 0/F:Aroot\C[Bdata-theme=D"2light"2\C]D 0\C.text-button:Ahover 0\C{B 0color:A 0var\C(--accent-strong\C);B 0\C}D/F);B
A}D);B
A
Atest("2æ6–6°0å5¢2Eå5¸8‚2å5œCºAã3€01æ6•5°0æ6D®Eä4¸8­Då5¿Fƒ3ä4¸8Eç7 0”4ç7©9¶6å5ŒCºAå5D—7ä4¿FDæ6ŒC1 012px 0è8¾E…5å5ŠA©9æ6–6‡7å5­D—7å5ŸFºAç7ºA¿F"2,C 0() 0=D>E 0{B
A 0 0const 0readableRules 0=D 0[B
A 0 0 0 0[B/F\C.market-extension-metric 0small,C\Cs*\C.market-extension-metric 0span\Cs*\C{B[B^E}D]D*font-size:A\Cs*12px;B/F,C 0"2market 0extension 0labels"2]D,C
A 0 0 0 0[B/F\C.market-planned-state 0p\Cs*\C{B[B^E}D]D*font-size:A\Cs*12px;B/F,C 0"2market 0planned-state 0copy"2]D,C
A 0 0 0 0[B/F\C.market-global-placeholder 0span\Cs*\C{B[B^E}D]D*font-size:A\Cs*12px;B/F,C 0"2global 0market 0placeholder 0copy"2]D,C
A 0 0 0 0[B/F\C.data-center-summary-counts 0small\Cs*\C{B[B^E}D]D*font-size:A\Cs*12px;B/F,C 0"2data-center 0summary 0labels"2]D,C
A 0 0 0 0[B/F\C.data-center-service-heading 0p\Cs*\C{B[B^E}D]D*font-size:A\Cs*12px;B/F,C 0"2data-center 0service 0descriptions"2]D,C
A 0 0 0 0[B/F\C.data-center-service-icon\Cs*\C{B[B^E}D]D*font-size:A\Cs*12px;B/F,C 0"2data-center 0service 0icon 0labels"2]D,C
A 0 0 0 0[B/F\C.data-center-check-row\Cs*\C{B[B^E}D]D*font-size:A\Cs*12px;B/F,C 0"2data-center 0check 0details"2]D,C
A 0 0 0 0[B/F\C.data-center-service-detail\Cs*\C{B[B^E}D]D*font-size:A\Cs*12px;B/F,C 0"2data-center 0service 0details"2]D,C
A 0 0 0 0[B/F\C.data-center-capability-status\Cs*\C{B[B^E}D]D*font-size:A\Cs*12px;B/F,C 0"2capability 0status 0labels"2]D,C
A 0 0 0 0[B/F\C.data-center-provider-row 0small\Cs*\C{B[B^E}D]D*font-size:A\Cs*12px;B/F,C 0"2provider 0descriptions"2]D,C
A 0 0 0 0[B/F\C.data-center-capability-tag\Cs*\C{B[B^E}D]D*font-size:A\Cs*12px;B/F,C 0"2capability 0tags"2]D,C
A 0 0 0 0[B/F\C.data-center-diagnostic-note\Cs*\C{B[B^E}D]D*font-size:A\Cs*12px;B/F,C 0"2diagnostic 0notes"2]D,C
A 0 0 0 0[B/F\C.research-list-symbol\Cs*\C{B[B^E}D]D*font-size:A\Cs*12px;B/F,C 0"2research 0list 0symbols"2]D,C
A 0 0 0 0[B/F\C.research-list-action\Cs*\C{B[B^E}D]D*font-size:A\Cs*12px;B/F,C 0"2research 0list 0actions"2]D,C
A 0 0 0 0[B/F\C.watchlist-tracking-line\Cs*\C{B[B^E}D]D*font-size:A\Cs*12px;B/F,C 0"2watchlist 0tracking 0details"2]D,C
A 0 0 0 0[B/F\C.research-overview-context 0span\Cs*\C{B[B^E}D]D*font-size:A\Cs*12px;B/F,C 0"2research 0overview 0context"2]D,C
A 0 0 0 0[B/F\C.research-overview-metric 0span\Cs*\C{B[B^E}D]D*font-size:A\Cs*12px;B/F,C 0"2research 0overview 0labels"2]D,C
A 0 0 0 0[B/F\C.research-overview-note\Cs*\C{B[B^E}D]D*font-size:A\Cs*12px;B/F,C 0"2research 0overview 0notes"2]D,C
A 0 0 0 0[B/F\C.research-detail-nav-label\Cs*\C{B[B^E}D]D*font-size:A\Cs*12px;B/F,C 0"2research 0detail 0navigation 0labels"2]D
A 0 0]D;B
A 0 0for 0(const 0[Brule,C 0description]D 0of 0readableRules) 0{B
A 0 0 0 0assert.match(stylesheet,C 0rule,C 0description 0+B 0"2 0must 0respect 0the 012px 0support-text 0baseline"2);B
A 0 0}D
A}D);B
A
Atest("2ç7 0”4ç7©9¶6ä4¸8Eè8®E¾Eç7½D®Eå5ˆ8†6å5ŒCºAå5¯F¼Cè8ˆ8ªAä4¼CšAç7§7»Bå5ŠA¨8é9”4®Eç7›B˜8ç7„4¦6ç7‚2¹9å5¹9¶6æ6ŒC‰9å5…5¨8é9ƒ3¨8å5F¯Fè8§71å5ŒCºAå5D—7å50ŒCæ6­D¥5é9«B˜8ä4ºA®E"2,C 0() 0=D>E 0{B
A 0 0assert.match(
A 0 0 0 0app,C
A 0 0 0 0/Ffunction 0bindSectionNavigation\C([B\Cs\CS]D*?FfocusTarget\C.focus\C(\C{B 0preventScroll:A 0true 0\C}D\C)[B\Cs\CS]D*?Fconst 0current 0=D 0targets[B\Cs\CS]D*?FgetBoundingClientRect\C(\C)/F
A 0 0);B
A}D);B
A
Atest("2å5ˆ8†6å5ŒCºAå5¯F¼Cè8ˆ8ªAç7šA„4ç7¨8‹Bå5ºAFå5ŒC–6ç7„4¦6ç7‚2¹9å5œC¨8ç7 0”4ç7©9¶6å5’2ŒCè8®E¾Eç7½D®Eé9¡1µ5ä4¸8­Då5F¯Fè8§71"2,C 0() 0=D>E 0{B
A 0 0assert.match(
A 0 0 0 0stylesheet,C
A 0 0 0 0/F#3settings-view 0\C.settings-section 0h2:Afocus,C[B\Cs\CS]D*?F#3research-view 0\C.supplemental-card 0h3:Afocus\Cs*\C{B[B^E}D]D*outline:A\Cs*2px 0solid 0var\C(--accent\C)/F
A 0 0);B
A}D);B
A
Atest("2å5¯F¼Cå5…5¥5æ6µ5Fè8§7ˆ8å5™9¨8å5·7¥5ä4½DœCå5ŒCºAä4¸8Dä4¼CšAæ6ŠAŠAå5¯F¼Cå5…5¥5å5‰9Dç7šA„4æ6´4»Bå5ŠA¨8ä4»B»Bå5ŠA¡1é9‡7Dæ6–6°0å5†6™9å5›BEä4»B»Bå5ŠA¡1å5¼C•5ç7”4¨8"2,C 0() 0=D>E 0{B
A 0 0assert.match(
A 0 0 0 0app,C
A 0 0 0 0/Ffunction 0applySettings\C(nextSettings,C 0\C{B 0persistPreviousActiveTask 0=D 0true 0\C}D 0=D 0\C{B\C}D\C)/F
A 0 0);B
A 0 0const 0importBody 0=D 0app.match(/Fasync 0function 0applyBrowserDataImport\C(imported,C 0currentWatchlist,C 0currentRecentTasks,C 0strategy\C) 0\C{B[B\Cs\CS]D*?F\C\Cn\C}D/F)?F.[B0]D 0|C|C 0"2"2;B
A 0 0assert.match(
A 0 0 0 0importBody,C
A 0 0 0 0/FlocalStorage\C.removeItem\C(TASK_REFERENCE_KEY\C);B[B\Cs\CS]D*?FapplySettings\C(nextSettings,C 0\C{B 0persistPreviousActiveTask:A 0false 0\C}D\C);B[B\Cs\CS]D*?FapplyTheme\C(imported\C.theme\C)/F
A 0 0);B
A 0 0assert.match(importBody,C 0/Fconst 0importedActiveTaskId 0=D 0state\C.taskId;B[B\Cs\CS]D*?Fconst 0importedActiveTaskBase 0=D 0state\C.taskAnalysisBase 0\C|C\C|C 0state\C.settings\C.analysisBase;B/F);B
A 0 0assert.match(importBody,C 0/FapplySettings\C(nextSettings,C 0\C{B 0persistPreviousActiveTask:A 0false 0\C}D\C);B[B\Cs\CS]D*?Fstate\C.suppressedHistoryTaskId 0=D 0importedActiveTaskId;B[B\Cs\CS]D*?Fstate\C.suppressedHistoryTaskBase 0=D 0importedActiveTaskId 0\C?F 0importedActiveTaskBase 0:A 0"2"2;B/F,C
A 0 0 0 0"2å5¯F¼Cå5…5¥5å5·7¥5ä4½DœCå5ŒCºAå50EïF¼CŒCæ6—7§7æ6´4»Bå5ŠA¨8ä4»B»Bå5ŠA¡1ä4¸8Dè8ƒ3½Då5œC¨8å50Eç7»B­Dè8½D®Eè8¯F¢2ä4¸8­Dé9‡7Dæ6–6°0å5†6™9å5›BEæ6œC¬Cå5œC°0å5E†6å5F²2"2);B
A 0 0const 0applyIndex 0=D 0importBody.indexOf("2applySettings(nextSettings,C 0{B 0persistPreviousActiveTask:A 0false 0}D)"2);B
A 0 0assert.ok(applyIndex 0>E=D 00,C 0"2workspace 0import 0must 0apply 0the 0validated 0settings"2);B
A 0 0const 0rollbackIndex 0=D 0importBody.indexOf("2const 0rollbackFailures 0=D 0[B]D"2);B
A 0 0assert.ok(rollbackIndex 0>E 00 0&6&6 0rollbackIndex 0<C 0applyIndex,C
A 0 0 0 0"2storage 0rollback 0must 0finish 0before 0the 0in-memory 0settings 0are 0applied"2);B
A 0 0assert.match(importBody.slice(rollbackIndex,C 0applyIndex),C
A 0 0 0 0/FlocalStorage\C.(?F:AsetItem|CremoveItem)\C([B\Cs\CS]D*?FlocalStorage\C.getItem\C(key\C) 0!=D=D 0\C(value 0\C?F\C?F 0null\C)[B\Cs\CS]D*?FrollbackFailures\C.push\C(key\C)/F,C
A 0 0 0 0"2rollback 0must 0read 0back 0every 0restored 0key 0and 0report 0silent 0failures"2);B
A 0 0const 0uiFailureIndex 0=D 0importBody.indexOf("2é9¡1µ5é9D¢2ç7ŠA¶6æ6€01å5ˆ8·7æ6–6°0æ6œCªAå5®EŒCæ6ˆ80"2,C 0applyIndex);B
A 0 0assert.ok(uiFailureIndex 0>E 0applyIndex,C 0"2UI 0refresh 0failures 0must 0be 0reported 0separately 0after 0storage 0commit"2);B
A 0 0assert.doesNotMatch(
A 0 0 0 0importBody.slice(applyIndex,C 0uiFailureIndex),C
A 0 0 0 0/FlocalStorage\C.(?F:AsetItem|CremoveItem)\C(|CpersistTheme\C(/F,C
A 0 0 0 0"2post-commit 0UI 0refresh 0must 0not 0mutate 0or 0roll 0back 0the 0imported 0storage"2
A 0 0);B
A 0 0assert.match(
A 0 0 0 0importBody,C
A 0 0 0 0/Fconst 0expectedStorage 0=D 0new 0Map\C(\C[B[B\Cs\CS]D*?F\C[BTASK_REFERENCE_KEY,C 0null\C]D[B\Cs\CS]D*?Ffor 0\C(const 0\C[Bkey,C 0expectedValue\C]D 0of 0expectedStorage\C)[B\Cs\CS]D*?FlocalStorage\C.getItem\C(key\C) 0!=D=D 0expectedValue[B\Cs\CS]D*?Fthrow 0new 0Error\C("2æ6µ5Fè8§7ˆ8å5™9¨8å5­D˜8å5‚2¨8æ6œCªAè8ƒ3½Dç7¡1®Eè8®E¤4å5†6™9å5…5¥5ïF¼CšA"2/F,C
A 0 0 0 0"2workspace 0import 0must 0verify 0every 0stored 0value 0before 0changing 0in-memory 0state"2
A 0 0);B
A 0 0assert.match(importBody.slice(rollbackIndex),C 0/FstorageKeys\C.forEach\C([B\Cs\CS]D*?FlocalStorage\C.(?F:AsetItem|CremoveItem)\C(/F,C
A 0 0 0 0"2the 0failure 0path 0should 0attempt 0to 0restore 0the 0pre-import 0storage 0snapshot"2
A 0 0);B
A}D);B
A
Atest("2å5ˆ8‡7æ6D¢2å5ˆ8°0å5°0šAæ6œCªAæ6E¥5å5…5¥5ç7šA„4æ6D¿Få5D—7ç7»B´4å5ºA¦6æ6—7¶6ïF¼CŒCæ6—7§7è8¯F·7æ6±1‚2ä4¸8Dè8ƒ3½Dè8¦6†6ç7›B–6è8ƒ3½Då5ŠA›Bå5D 0ä4½DD"2,C 0() 0=D>E 0{B
A 0 0assert.match(
A 0 0 0 0marketOverview,C
A 0 0 0 0/Fasync 0function 0load\C(key\C)\Cs*\C{B[B\Cs\CS]D*?Fcancel\C(key\C);B\Cs*seq=Dgeneration\C[Bkey\C]D;B[B\Cs\CS]D*?Fif\C(key=D=D=D"2sectors"2&6&6sectorMetric!=D=D"2change"2\C)\C{BshowSectorMetricUnavailable\C(\C);Breturn 0"2unsupported"2;B\C}D/F
A 0 0);B
A 0 0assert.match(
A 0 0 0 0marketOverview,C
A 0 0 0 0/FsectorType=Dbutton\C.dataset\C.sectorType=D=D=D"2concept"2\C?F"2concept"2:A"2industry"2[B\Cs\CS]D*?Fvoid 0load\C("2sectors"2\C)/F
A 0 0);B
A 0 0assert.match(
A 0 0 0 0marketOverview,C
A 0 0 0 0/FsectorMetric=D\C[B"2change"2,C"2amount"2,C"2five-day"2,C"2flow"2\C]D[B\Cs\CS]D*?Fvoid 0load\C("2sectors"2\C)/F
A 0 0);B
A}D);B
A
Atest("2å5¿F«Bç7…5§7æ6D¥5æ6ºA0ã3€01ä4¼C°0å5€0¼Cä4¸8Eè8¡1¥5å5…5…5æ6ŒC‡7æ6 0‡7ä4½D¿Fç7”4¨8å5F¯Fè8¯F»Bç7šA„4 012px 0è8¾E…5å5ŠA©9æ6–6‡7å5­D—7"2,C 0() 0=D>E 0{B
A 0 0const 0readableRules 0=D 0[B
A 0 0 0 0[B/F\C.bundle-snapshot-metrics 0small\Cs*\C{B[B^E}D]D*font-size:A\Cs*12px;B/F,C 0"2snapshot 0metadata 0labels"2]D,C
A 0 0 0 0[B/F\C.bundle-snapshot-source 0small\Cs*\C{B[B^E}D]D*font-size:A\Cs*12px;B/F,C 0"2snapshot 0source 0labels"2]D,C
A 0 0 0 0[B/F\C.bundle-snapshot-source 0strong\Cs*\C{B[B^E}D]D*font-size:A\Cs*12px;B/F,C 0"2snapshot 0source 0names"2]D,C
A 0 0 0 0[B/F\C.stock-valuation-metric 0>E 0span\Cs*\C{B[B^E}D]D*font-size:A\Cs*12px;B/F,C 0"2valuation 0metric 0labels"2]D,C
A 0 0 0 0[B/F\C.stock-supplement-metric 0>E 0span\Cs*\C{B[B^E}D]D*font-size:A\Cs*12px;B/F,C 0"2stock 0supplement 0metric 0labels"2]D
A 0 0]D;B
A 0 0for 0(const 0[Brule,C 0description]D 0of 0readableRules) 0{B
A 0 0 0 0assert.match(stylesheet,C 0rule,C 0description 0+B 0"2 0must 0use 0at 0least 012px 0text"2);B
A 0 0}D
A}D);B
A
Atest("2æ6•5°0æ6D®Eä4¸8­Då5¿Fƒ3æ6£3€0æ6ŸF¥5å5ˆ8Då5§7‹Bå5ŒC–6ä4¸8E 0finally 0è8§7£3é9”41å5¼C‚2å5¸8¸8ä4¸8Dä4¼CšAç7 0´4å5DF 0generation 0æ6”4¶6å5°0¾E"2,C 0() 0=D>E 0{B
A 0 0const 0refresh 0=D 0dataCenter.match(/Fasync 0function 0refresh\C(\C) 0\C{B[B\Cs\CS]D*?F\C\Cn 0 0\C}D/F)?F.[B0]D 0|C|C 0"2"2;B
A 0 0assert.ok(refresh);B
A 0 0assert.match(refresh,C 0/Fchecking 0=D 0true;B[B\Cs\CS]D*?Ftry 0\C{B\Cs*settings 0=D 0typeof 0getSettings/F);B
A 0 0assert.match(refresh,C 0/Fæ6ŒC‰9é9’2®E 0DOM 0æ6›B´4æ6–6°0å5¤4±1è8´4¥5ä4¸8Dè8ƒ3½Dé9˜8»Bæ6­D¢2æ6œCDå5ŠA¡1æ6E¢2æ6µ5‹B[B\Cs\CS]D*?Fawait 0Promise\C.all/F);B
A 0 0assert.match(refresh,C 0/Fcatch 0\C(error\C) 0\C{B\Cs*if 0\C(current 0!=D=D 0generation\C) 0return;B[B\Cs\CS]D*?FsafeError\C(error\C)/F);B
A 0 0assert.match(refresh,C 0/Ffinally 0\C{B[B\Cs\CS]D*?Fchecking 0=D 0false;B[B\Cs\CS]D*?Fbutton\C.disabled 0=D 0false;B[B\Cs\CS]D*?Ffinally 0ä4ºA§7ç7”4ŸFæ6–6°0ç7šA„4 0rejection/F);B
A 0 0const 0invalidate 0=D 0dataCenter.match(/Ffunction 0invalidate\C(message 0=D 0[B\Cs\CS]D*?F\C\Cn 0 0\C}D/F)?F.[B0]D 0|C|C 0"2"2;B
A 0 0assert.match(invalidate,C 0/Fgeneration 0\C+B=D 01;B[B\Cs\CS]D*?FsafeInvalidateStep/F);B
A 0 0assert.match(invalidate,C 0/FsafeInvalidateStep\C(\C(\C) 0=D>E 0setBadge\C("2#3data-center-summary-status"2/F);B
A 0 0assert.match(invalidate,C 0/FforEach\C(\C(selector\C) 0=D>E 0safeInvalidateStep/F);B
A}D);B
A
Atest("2æ6•5°0æ6D®Eä4¸8­Då5¿Fƒ3è8¶6…5æ6—7¶6å50Eå5D³3ä4½D¿Fæ6E¢2é9’2ˆ8è8¿FŸFå5ˆ8°0è8¿F”4å5›BEïF¼CŒCä4¹9ŸFä4¸8Dè8ƒ3½Dè8¦6†6ç7›B–6ä4¸8ºAæ6ˆ80å5ŠAŸF"2,C 0() 0=D>E 0{B
A 0 0assert.match(
A 0 0 0 0healthCheck,C
A 0 0 0 0/F\C.then\C(\C(data\C) 0=D>E 0timedOut\Cs*\C?F 0\C{B 0ok:A 0false,C 0error:A 0new 0Error\C("2æ6£3€0æ6ŸF¥5è8¶6…5æ6—7¶6"2\C),C 0timedOut:A 0true/F
A 0 0);B
A}D);B
A
Atest("2è8¡1ŒCæ6ƒ3…5æ6ŸF¥5è8¯F¢2å5ˆ8Då5§7‹Bå5ŒC–6ä4¸8E 0finally 0æ6¸8²2æ6ŸF“3å5¼C‚2å5¸8¸8ä4¸8Dä4¼CšAé91—7ç7•5™9æ6ŸF¥5è8¯F¢2æ6ŒC‰9é9’2®Eé9”41"2,C 0() 0=D>E 0{B
A 0 0const 0start 0=D 0app.indexOf("2async 0function 0runQuoteQuery(event) 0{B"2);B
A 0 0const 0end 0=D 0app.indexOf("2\C\Cnfunction 0"2,C 0start 0+B 020);B
A 0 0const 0query 0=D 0start 0>E=D 00 0&6&6 0end 0>E 0start 0?F 0app.slice(start,C 0end) 0:A 0"2"2;B
A 0 0const 0tryStart 0=D 0query.indexOf("2try 0{B"2,C 0query.indexOf("2const 0generation 0=D 0+B+Bstate.quoteQueryGeneration;B"2));B
A 0 0assert.ok(tryStart 0>E=D 00,C 0"2è8¯F·7æ6±1‚2ç7”4ŸFå5‘1½Då5‘1¨8æ6œCŸFå5¿F…5é9¡1»Bæ6œC‰9ä4¸8»B 0try"2);B
A 0 0assert.ok(query.indexOf('state.quoteQueryController 0=D 0controller;B',C 0tryStart) 0>E 0tryStart);B
A 0 0assert.ok(query.indexOf('$4("2#3quote-form"2).classList.add("2is-loading"2);B',C 0tryStart) 0>E 0tryStart);B
A 0 0assert.match(query,C 0/Fcatch 0\C(error\C) 0\C{B[B\Cs\CS]D*?Fæ6¸8…5ç70†6æ6—7§7è8¡1ŒCæ6ƒ3…5æ6˜8¾Eç7¤4ºAå5¤4±1è8´4¥5ïF¼CšA[B\Cs\CS]D*?Ffinally 0\C{B/F);B
A 0 0const 0finallyStart 0=D 0query.lastIndexOf("2}D 0finally 0{B"2);B
A 0 0const 0finalizer 0=D 0query.slice(finallyStart);B
A 0 0assert.match(finalizer,C 0/Fstate\C.isQuerying 0=D 0false;B[B\Cs\CS]D*?Ftry 0\C{B\Cs*renderResearchOverview\C(\C);B\Cs*\C}D 0catch/F);B
A 0 0assert.match(finalizer,C 0/Fcatch 0\C(renderError\C)[B\Cs\CS]D*?Ftry 0\C{B\Cs*releaseQuoteQueryControls\C(\C);B\Cs*\C}D 0catch 0\C(unlockError\C)/F);B
A 0 0assert.match(finalizer,C 0/Fè8¡1ŒCæ6ƒ3…5è8¯F·7æ6±1‚2å5·7²2ç7»B“3æ6DŸFïF¼CŒCä4½D†6æ6ŸF¥5è8¯F¢2æ6ŒC‰9é9’2®Eç7ŠA¶6æ6€01æ61¢2å5¤4Då5¤4±1è8´4¥5/F);B
A}D);B
A
Atest("2å5¸8‚2å5œCºAå5·7¥5ä4½DœCå5F°0å50„4æ6¨8¡1å5D—7å5ˆ8·7æ6–6°0å5¤4±1è8´4¥5ä4¼CšAç7‹B¬Cç7«B‹Bæ6”4¶6æ6•5›Bå5¹9¶6æ6˜8¾Eç7¤4ºAè8¯FŠAæ6–6­D"2,C 0() 0=D>E 0{B
A 0 0const 0refresh 0=D 0app.match(/Ffunction 0refreshMarketWorkspace\C(\C) 0\C{B[B\Cs\CS]D*?F\C\Cn\C}D/F)?F.[B0]D 0|C|C 0"2"2;B
A 0 0assert.ok(refresh,C 0"2refreshMarketWorkspace 0implementation 0must 0exist"2);B
A 0 0assert.match(refresh,C 0/Fconst 0startModule 0=D 0\C(label,C 0operation\C) 0=D>E 0\C{B[B\Cs\CS]D*?FPromise\C.resolve\C(operation\C(\C)\C)\C.then/F);B
A 0 0assert.match(refresh,C 0/F\C(error\C) 0=D>E 0\C(\C{B 0label,C 0ok:A 0false,C 0error 0\C}D\C)/F);B
A 0 0assert.match(refresh,C 0/Fcatch 0\C(error\C) 0\C{B[B\Cs\CS]D*?Freturn 0Promise\C.resolve\C(\C{B 0label,C 0ok:A 0false,C 0error 0\C}D\C)/F);B
A 0 0assert.match(refresh,C 0/FstartModule\C("2å5¸8‚2å5œCºAæ6€0»Bè8§7ˆ8"2[B\Cs\CS]D*?FstartModule\C("2è8µ5„4é9‡7‘1ä4¸8Eæ6´4»Bè8·7ƒ3å5ºA¦6"2/F);B
A 0 0assert.match(refresh,C 0/FPromise\C.all\C(requests\C)\C.then\C(\C(results\C) 0=D>E 0\C{B[B\Cs\CS]D*?Fresult\C.ok[B\Cs\CS]D*?Få5¸8‚2å5œCºAå5·7¥5ä4½DœCå5F°0é9ƒ3¨8å5ˆ8†6æ6¨8¡1å5D—7å5ˆ8·7æ6–6°0å5¤4±1è8´4¥5/F);B
A 0 0assert.match(refresh,C 0/Få5¸8‚2å5œCºAå5·7¥5ä4½DœCå5F°0å5ˆ8·7æ6–6°0æ6±1‡7æ6€0»Bå5¼C‚2å5¸8¸8ïF¼CšA[B\Cs\CS]D*?Freturn 0\C[B\C]D/F);B
A}D);B
A
Atest("2æ6œCDå5ŠA¡1å51¥5å5ºA·7å5ˆ8·7æ6–6°0æ6ŠAŠAè8¯F·7æ6±1‚2ä4¸8E 0DOM 0å5¼C‚2å5¸8¸8æ6”4¶6æ6•5›Bä4¸8ºAç7»B“3æ6EœC"2,C 0() 0=D>E 0{B
A 0 0const 0refresh 0=D 0app.match(/Fasync 0function 0refreshServices\C(\C) 0\C{B[B\Cs\CS]D*?F\C\Cn\C}D/F)?F.[B0]D 0|C|C 0"2"2;B
A 0 0assert.ok(refresh,C 0"2refreshServices 0implementation 0must 0exist"2);B
A 0 0assert.match(refresh,C 0/Ftry 0\C{B[B\Cs\CS]D*?Fawait 0Promise\C.all\C(\C[B[B\Cs\CS]D*?FrunHealthCheck\C(marketHealth,C 0settings\C)/F);B
A 0 0assert.match(refresh,C 0/Fcatch 0\C(renderError\C)[B\Cs\CS]D*?Fç7ŠA¶6æ6€01æ6˜8¾Eç7¤4ºAæ6›B´4æ6–6°0å5¤4±1è8´4¥5[B\Cs\CS]D*?Freturn 0\C{B 0marketOK,C 0analysisOK,C 0marketState,C 0analysisState,C 0stale:A 0false,C 0renderError:A 0message 0\C}D/F);B
A 0 0assert.match(refresh,C 0/Fcatch 0\C(error\C)[B\Cs\CS]D*?Fæ6œCDå5ŠA¡1å51¥5å5ºA·7æ6£3€0æ6ŸF¥5æ6µ51ç7¨8‹Bå5¼C‚2å5¸8¸8[B\Cs\CS]D*?Freturn 0\C{B 0\C.\C.\C.staleResult,C 0stale:A 0false,C 0error:A 0message 0\C}D/F);B
A 0 0assert.match(refresh,C 0/Fgeneration 0!=D=D 0serviceRefreshGeneration/F);B
A 0 0assert.match(refresh,C 0/Flabel\C.title 0=D 0message/F);B
A}D);B
A
Atest("2æ6œCDå5ŠA¡1å51¥5å5ºA·7å5ˆ8·7æ6–6°0ä4¸8Dä4¼CšAè8¦6†6ç7›B–6è8¡1ŒCæ6ƒ3…5æ6ŸF¥5è8¯F¢2å5ŒCºAå5ŸFŸFç7šA„4ç7‹B¬Cç7«B‹Bæ6F0ç7¤4ºA"2,C 0() 0=D>E 0{B
A 0 0const 0refreshServices 0=D 0app.match(/Fasync 0function 0refreshServices\C(\C) 0\C{B[B\Cs\CS]D*?F\C\Cn\C}D/F)?F.[B0]D 0|C|C 0"2"2;B
A 0 0assert.ok(refreshServices,C 0"2refreshServices 0implementation 0must 0exist"2);B
A 0 0assert.doesNotMatch(
A 0 0 0 0refreshServices,C
A 0 0 0 0/F#3query-hint/F,C
A 0 0 0 0"2service 0health 0feedback 0belongs 0to 0service 0indicators,C 0not 0the 0current 0stock 0query 0hint"2
A 0 0);B
A}D);B
A
Atest("2ä4¾E§7æ6 0Fä4¸8Eé9¡1¶6æ6 0Fæ6œCDå5ŠA¡1ç7ŠA¶6æ6€01å5FªAæ6œC‰9å5œC¨8æ6˜8Eç7¡1®Eå51¥5å5ºA·7å5›BEå5ŒC…5æ6—7¶6æ6‰9Dæ6˜8¾Eç7¤4ºAæ6­D£3å5¸8¸8"2,C 0() 0=D>E 0{B
A 0 0assert.ok(app.includes('if 0(!result?F.ok) 0return 0"2failed"2;B'));B
A 0 0assert.ok(app.includes('const 0status 0=D 0String(result.data?F.status 0|C|C 0"2"2).trim().toLowerCase();B'));B
A 0 0assert.match(app,C 0/Ffunction 0serviceHealthState\C(result\C)[B\Cs\CS]D*?Freturn 0"2unknown"2;B/F);B
A 0 0assert.match(app,C 0/FsetText\C("2#3connection-label"2,C 0connectionLabel\C)/F);B
A 0 0assert.doesNotMatch(app,C 0/FsetServiceStatus\C(/F,C 0"2service 0refresh 0must 0not 0reference 0removed 0per-service 0sidebar 0elements"2);B
A 0 0assert.match(stylesheet,C 0/F\C.status-dot\C.is-unknown\Cs*\C{B[B^E}D]D*background:A\Cs*var\C(--text-dim\C)/F);B
A}D);B
A
Atest("2æ6•5°0æ6D®Eä4¸8­Då5¿Fƒ3ä4¸8Dä4¼CšAæ6ŠAŠAç7¼CºAå5°0‘1 0status 0ç7šA„4 02xx 0å5›BEå5ŒC…5æ6 0‡7è8®E°0ä4¸8ºAå51¥5å5ºA·7"2,C 0() 0=D>E 0{B
A 0 0assert.match(dataCenter,C 0/Fif 0\C(!raw\C) 0return 0"2unknown"2;B/F);B
A 0 0assert.match(
A 0 0 0 0dataCenter,C
A 0 0 0 0/Fcurrent 0=D=D=D 0"2degraded"2 0\C|C\C|C 0current 0=D=D=D 0"2unknown"2 0\C?F 0"2is-warning"2 0:A 0"2is-error"2/F
A 0 0);B
A 0 0assert.match(dataCenter,C 0/Fconst 0unknownServices 0=D 0serviceStates\C.filter\C(\C(status\C) 0=D>E 0status 0=D=D=D 0"2unknown"2\C)\C.length;B/F);B
A 0 0assert.match(dataCenter,C 0/Fconst 0unknown 0=D 0unknownServices 0\C+B 0unknownDependencies/F);B
A}D);B
A
A
Atest("2æ6œC€0è8¿F‘1ä4»B»Bå5ŠA¡1æ6œC¬Cå5œC°0å5­D˜8å5‚2¨8ä4¸8Då5F¯Fè8¯F»Bæ6ˆ8–6æ6DŸFå5DFæ6—7¶6ä4¸8Dè8¦6†6ç7›B–6å5EŸFå5§7‹Bå5E†6å5F²2ïF¼CŒCå5¯F¼Cå5‡7ºAä4¸8Dä4¼CšAé9D™9é9»B˜8é91—7æ6¼CFè8®E°0å5½D•5"2,C 0() 0=D>E 0{B
A 0 0assert.match(recentTaskModule,C 0/Fexport 0function 0readRecentTasksState\C(storage,C 0limit 0=D 0MAX_RECENT_TASKS\C)/F);B
A 0 0assert.match(recentTaskModule,C 0/Fif 0\C(!currentState\C.storageReadable 0\C|C\C|C 0!currentState\C.storageValid\C) 0return 0currentState\C.items/F);B
A 0 0assert.match(app,C 0/Ffunction 0safeReadRecentTasksState\C(\C)/F);B
A 0 0assert.match(app,C 0/Ffunction 0renderRecentTasks\C(records\C)[B\Cs\CS]D*?FhistoryState\C.storageReadable[B\Cs\CS]D*?FhistoryState\C.storageValid[B\Cs\CS]D*?Få5E†6å5F²2ä4¿F®Eæ6”4¹9å5·7²2æ6šA‚2å51œC/F);B
A 0 0assert.match(app,C 0/Frecent_tasks:A 0\C(\C(\C) 0=D>E 0\C{B[B\Cs\CS]D*?FrecentTasksState\C.storageReadable[B\Cs\CS]D*?FrecentTasksState\C.storageValid[B\Cs\CS]D*?Få5¯F¼Cå5‡7ºAå5·7²2å5F–6æ6¶6ˆ8/F);B
A}D);B
A
A
Atest("2æ6œC€0è8¿F‘1ä4»B»Bå5ŠA¡1æ6–6°0å5¢2Eä4¸8Eå5ˆ8 0é9™9¤4å5¿F…5é9¡1»Bç7¡1®Eè8®E¤4å5®EŒCæ6•5´4è8®E°0å5½D•5å5·7²2æ6ŒC1ä4¹9…5å5ŒC–6ïF¼CŒCå5¤4±1è8´4¥5æ6—7¶6ä4¿FDç7•5™9å5EŸFå5ˆ8—7è8¡1¨8"2,C 0() 0=D>E 0{B
A 0 0assert.match(recentTaskModule,C 0/Fstorage\C.setItem\C(RECENT_TASKS_KEY,C 0JSON\C.stringify\C(records\C)\C)[B\Cs\CS]D*?Fconst 0persistedState 0=D 0readRecentTasksState\C(storage,C 0limit\C)[B\Cs\CS]D*?Freturn 0persisted 0\C?F 0persistedState\C.items 0:A 0currentState\C.items/F);B
A 0 0assert.match(recentTaskModule,C 0/Fif 0\C(JSON\C.stringify\C(persistedState\C.items\C) 0!=D=D 0JSON\C.stringify\C(records\C)\C) 0return 0currentState\C.items/F);B
A 0 0assert.match(recentTaskModule,C 0/Få5ˆ8 0é9™9¤4æ6œCªAæ6ˆ80å5ŠAŸFæ6ŒC1ä4¹9…5å5ŒC–6æ6—7¶6ä4¿FDç7•5™9å5EŸFå5ˆ8—7è8¡1¨8/F);B
A 0 0assert.match(app,C 0/Ffunction 0persistRecentTaskHistory\C(record\C)[B\Cs\CS]D*?Fconst 0expected 0=D 0\C{B[B\Cs\CS]D*?FJSON\C.stringify\C(item\C.symbols\C) 0=D=D=D 0JSON\C.stringify\C(expected\C.symbols\C)[B\Cs\CS]D*?Fæ6œC€0è8¿F‘1ä4»B»Bå5ŠA¡1å5¼C•5ç7”4¨8æ6œCªAè8ƒ3½Dç7¡1®Eè8®E¤4æ6ŒC1ä4¹9…5å5ŒC–6/F);B
A}D);B
A
A
Atest("2æ6´4»Bå5ŠA¨8ä4»B»Bå5ŠA¡1å5¼C•5ç7”4¨8é9D™9é9»B˜8å5†6™9å5…5¥5å5¤4±1è8´4¥5æ6—7¶6ä4»BDç7‹B¬Cç7«B‹Bä4¿FDå5­D˜8æ6œC€0è8¿F‘1ä4»B»Bå5ŠA¡1å5E†6å5F²2å5¹9¶6æ6˜8Eç7¡1®Eæ6F0ç7¤4ºA"2,C 0() 0=D>E 0{B
A 0 0assert.match(
A 0 0 0 0app,C
A 0 0 0 0/Fconst 0serializedReference 0=D 0JSON\C.stringify\C(record\C);B[B\Cs\CS]D*?FlocalStorage\C.setItem\C(TASK_REFERENCE_KEY,C 0serializedReference\C);B[B\Cs\CS]D*?FtaskReferencePersisted 0=D 0localStorage\C.getItem\C(TASK_REFERENCE_KEY\C) 0=D=D=D 0serializedReference/F
A 0 0);B
A 0 0assert.match(
A 0 0 0 0app,C
A 0 0 0 0/Fconst 0historyPersisted 0=D 0persistRecentTaskHistory\C(record\C);B[B\Cs\CS]D*?Fif 0\C(!taskReferencePersisted\C)[B\Cs\CS]D*?Fæ6œC€0è8¿F‘1ä4»B»Bå5ŠA¡1å5·7²2ä4¿FDå5­D˜8ïF¼CŒCä4½D†6æ6´4»Bå5ŠA¨8ä4»B»Bå5ŠA¡1å5¼C•5ç7”4¨8æ6œCªAè8ƒ3½Dç7¡1®Eè8®E¤4æ6ŒC1ä4¹9…5å5ŒC–6/F
A 0 0);B
A}D);B
A
Atest("2ä4¼C°0å5€0¼Cè8¯F·7æ6±1‚2è8¡1¥5é9½D0ä4ºA¤4æ6˜8“3æ6‰9€0å50Eç7¼C€0ïF¼CŒCå5¹9¶6å5FªAå5±1•5ç7¤4ºAä4¸8Eå5½D“3å5‰9Dæ6 0‡7ç7šA„4å5ŒC¹9é9…5Dç7šA„4è8®E°0å5½D•5"2,C 0() 0=D>E 0{B
A 0 0const 0renderer 0=D 0stockResearch.match(/Ffunction 0renderValuations\C(data,Csymbol\C)\C{B[B\Cs\CS]D*?F\C\Cn 0\C}D
A 0function 0fractionPercent/F)?F.[B0]D 0|C|C 0"2"2;B
A 0 0assert.ok(renderer,C 0"2ä4¼C°0å5€0¼Cæ6¸8²2æ6ŸF“3å5™9¨8å5ºA”4å5­D˜8å5œC¨8"2);B
A 0 0assert.match(stockResearch,C 0/Ffunction 0normalizeValuationSymbol\C(input\C)/F);B
A 0 0assert.match(stockResearch,C 0/Fimport 0\C{B 0comparableMarketSymbol,C 0.* 0from 0"2\C.\C/Fformat\C.js"2/F);B
A 0 0assert.match(stockResearch,C 0/Fconst 0normalized=DcomparableMarketSymbol\C(value\C)/F);B
A 0 0assert.match(stockResearch,C 0/Fif\C(!value\C.includes\C("2\C."2\C)&6&6normalized=D=D=Dvalue\C)[B\Cs\CS]D*?Fæ6—7 0æ6³3•5ä4»BEè8¯F¥5å5…5­Dä4½DDä4»B£3ç7 01å5ˆ8¤4æ6–6­Dä4ºA¤4æ6˜8“3æ6‰9€0/F);B
A 0 0assert.ok(stockResearch.includes("2getMarketValuations(settings,C[BnormalizeValuationSymbol(sym)]D,Csignal)"2));B
A 0 0assert.ok(renderer.includes('const 0requestedSymbol=DString(symbol|C|Cdata.requested_symbols?F.[B0]D|C|C"2"2).trim();B'));B
A 0 0assert.match(renderer,C 0/Fconst 0targetSymbol=DnormalizeValuationSymbol\C(requestedSymbol\C)/F);B
A 0 0assert.match(renderer,C 0/Fitems\C.find\C(value=D>EsameValuationSymbol\C(value\C.symbol,CtargetSymbol\C)\C)/F);B
A 0 0assert.doesNotMatch(renderer,C 0/Fitems\C[B0\C]D/F,C 0"2ç7›B®Eæ6 0‡7è8‚2¡1ç7¥5¨8æ6œCªAå5ŒC¹9é9…5Dæ6—7¶6ä4¸8Då5¾E—7é9€0€0å5›BEå5±1•5ç7¤4ºAå5ˆ8—7è8¡1¨8ç7¬C¬Cä4¸8€0æ6D¡1è8®E°0å5½D•5"2);B
A 0 0assert.match(renderer,C 0/Fä4¼C°0å5€0¼Cæ6E¥5å5F£3è8¿F”4å5›BEç7šA„4è8¯F1å5ˆ8¸8ä4»B£3ç7 01ä4¸8Eå5½D“3å5‰9Dæ6 0‡7ç7šA„4ä4¸8Då5ŒC¹9é9…5DïF¼CŒCå5·7²2å5¿F½Dç7•5¥5è8¿F™9ä4ºA›Bè8®E°0å5½D•5/F);B
A}D);B
A
A 0
Atest("2å5­D£3å5ºA¦6è8´4¢2å5ŠA¡1æ6ŒC‡7æ6 0‡7è8¯F»Bå5F–6ç7šA„4æ6ŠA¥5å5‘1ŠAæ6œCŸFæ6 0¡1é9ªAŒCä4¸8Eæ6ˆ80å5ŠAŸFæ6F0ç7¤4ºAå5¼C‚2å5¸8¸8ä4¸8Dä4¼CšAå50‘1å5¤4–6æ6‹B’2ç7»BD"2,C 0() 0=D>E 0{B
A 0 0const 0loader 0=D 0stockResearch.match(/Fasync 0function 0loadFundamentals\C(\C)\C{B[B\Cs\CS]D*?F\C\Cn 0\C}D/F)?F.[B0]D 0|C|C 0"2"2;B
A 0 0assert.ok(loader);B
A 0 0assert.match(loader,C 0/Ftry\C{B[B\Cs\CS]D*?FsyncSymbol\C(\C)/F);B
A 0 0assert.match(loader,C 0/Fif\C(!Number\C.isInteger\C(year\C)[B\Cs\CS]D*?FsafeResearchFeedback\C("2è8´4¢2å5ŠA¡1æ6ŠA¥5å5‘1ŠAæ6œCŸFä4¸8Dæ6­D£3ç7¡1®Eã3€0‚2"2\C)/F);B
A 0 0assert.match(loader,C 0/Fconst 0ok=Dawait 0fetchOne\C("2fundamentals"2[B\Cs\CS]D*?Få5­D£3å5ºA¦6è8´4¢2å5ŠA¡1æ6ŒC‡7æ6 0‡7å5·7²2è8¿F”4å5›BE/F);B
A 0 0assert.match(loader,C 0/Fcatch\C(error\C)\C{B\Cs*safeResearchFeedback\C("2å5­D£3å5ºA¦6è8´4¢2å5ŠA¡1æ6ŒC‡7æ6 0‡7è8¯F»Bå5F–6æ6µ51ç7¨8‹Bå5¼C‚2å5¸8¸8ïF¼CšA"2\C+BsafeResearchError\C(error\C)\C);B\Cs*return 0false;B/F);B
A}D);B
A
Atest("2ä4¸8ªAè8‚2¡1ç7 0”4ç7©9¶6å5‡7†6å5¤4‡7é9˜8¶6æ6®Eµ5å5¼C‚2å5¸8¸8ä4¸8E 0finally 0å5¼C‚2å5¸8¸8ä4¸8Dä4¼CšAé91—7ç7•5™9æ6ŒC‰9é9’2®Eé9”41"2,C 0() 0=D>E 0{B
A 0 0assert.match(stockResearch,C 0/Ffunction 0safeResearchButton\C(button,Cdisabled,Clabel\C)[B\Cs\CS]D*?Fcatch\C{Breturn 0false;B\C}D/F);B
A 0 0assert.match(stockResearch,C 0/Fasync 0function 0fetchOne\C(key,Crequest,Crender\C)[B\Cs\CS]D*?Ftry\C{B\Cs*abort\C(key\C);B[B\Cs\CS]D*?Fawait 0request\C(settings,Csymbol,Cctl\C.signal\C)[B\Cs\CS]D*?Ffinally\C{B/F);B
A 0 0assert.match(stockResearch,C 0/Fasync 0function 0loadResearch\C(\C)[B\Cs\CS]D*?FsafeResearchButton\C(button,Ctrue,C"2æ6­D£3å5œC¨8å5ŠA 0è8½D½Dâ2€0¦6"2[B\Cs\CS]D*?Ffinally\C{B[B\Cs\CS]D*?FsafeResearchButton\C(button,Cfalse,C"2é9‡7Dæ6–6°0å5ŠA 0è8½D½Då5…5¬Cå5F¸8è8µ5„4æ6–6™9"2\C)/F);B
A 0 0assert.match(stockResearch,C 0/Fasync 0function 0runSupplementRequest\C([B\Cs\CS]D*?Flet 0symbol=D"2"2,Cbutton=Dnull,Cgeneration=Dnull,CownsButton=Dfalse;B[B\Cs\CS]D*?Ftry\C{B[B\Cs\CS]D*?Fconst 0operation=DfetchOne\C(key,Crequest,Crender\C)[B\Cs\CS]D*?Ffinally\C{B[B\Cs\CS]D*?FsafeResearchButton\C(button,Cfalse,CidleText\C)/F);B
A 0 0assert.match(stockResearch,C 0/Fasync 0function 0loadAnnouncements\C(append=Dfalse\C)[B\Cs\CS]D*?Ftry\C{B[B\Cs\CS]D*?FsafeResearchButton\C(\C$4\C("2#3load-more-announcements"2\C),Ctrue,C"2æ6­D£3å5œC¨8å5ŠA 0è8½D½Dâ2€0¦6"2[B\Cs\CS]D*?Ffinally\C{B[B\Cs\CS]D*?FsafeResearchButton\C([B\Cs\CS]D*?F"2å5ŠA 0è8½D½Dæ6›B´4å5¤4šAå5…5¬Cå5‘1ŠA"2/F);B
A}D);B
A
Atest("2ä4¸8ªAè8‚2¡1ç7 0”4ç7©9¶6å5“3Då5ºA”4ä4»B£3ç7 01ä4¸8Då5ŒC¹9é9…5Dæ6—7¶6å5¿F½Dç7•5¥5ç7»B“3æ6EœC"2,C 0() 0=D>E 0{B
A 0 0assert.match(stockResearch,C 0/Ffunction 0assertResponseSymbol\C(data,CrequestedSymbol\C)/F);B
A 0 0assert.match(stockResearch,C 0/Fif\C(responseSymbol&6&6!sameValuationSymbol\C(responseSymbol,CrequestedSymbol\C)\C)/F);B
A 0 0assert.ok(stockResearch.includes('"2æ6œCDå5ŠA¡1ç7«B¯Fè8¿F”4å5›BEè8¯F1å5ˆ8¸8ä4»B£3ç7 01 0"2+BresponseSymbol+B"2 0ä4¸8Eè8¯F·7æ6±1‚2æ6 0‡7ç7šA„4 0"2+BrequestedSymbol+B"2 0ä4¸8Då5ŒC¹9é9…5DïF¼CŒCå5·7²2å5¿F½Dç7•5¥5ç7»B“3æ6EœCã3€0‚2'));B
A 0 0assert.match(stockResearch,C 0/Fasync 0function 0fetchOne\C(key,Crequest,Crender\C)[B\Cs\CS]D*?FassertResponseSymbol\C(data,Csymbol\C);B[B\Cs\CS]D*?Frender\C(data,Csymbol\C)/F);B
A 0 0assert.match(stockResearch,C 0/Fasync 0function 0loadAnnouncements\C(append=Dfalse\C)[B\Cs\CS]D*?FgetStockAnnouncements\C(settings,Csymbol,C[B\Cs\CS]D*?FassertResponseSymbol\C(data,Csymbol\C);B[B\Cs\CS]D*?Fconst 0root=D\C$4\C("2#3announcement-list"2\C)/F);B
A}D);B
A
A 0
Atest("2ä4¸8»Bè8¡1ŒCæ6ƒ3…5ã3€01æ6—7¥5 0K 0ä4¸8Eç7»BŸFä4¸8€0å5¿F«Bç7…5§7æ6‹B’2ç7»BDè8¯F1å5ˆ8¸8ä4»B£3ç7 01ä4¸8Då5ŒC¹9é9…5Dç7šA„4å5“3Då5ºA”4"2,C 0() 0=D>E 0{B
A 0 0assert.match(format,C 0/Fexport 0function 0comparableMarketSymbol\C(input\C)/F);B
A 0 0assert.match(app,C 0/Ffunction 0responseSymbolMismatch\C(payload,C 0requestedSymbol,C 0label\C)/F);B
A 0 0assert.match(app,C 0/FresponseSymbolMismatch\C(bundleResponse,C 0symbol,C 0"2ç7»BŸFä4¸8€0è8¡1ŒCæ6ƒ3…5å5¿F«Bç7…5§7"2\C)/F);B
A 0 0assert.match(app,C 0/FresponseSymbolMismatch\C(bundleResponse\C.quote,C 0symbol,C 0"2ç7»BŸFä4¸8€0å5¿F«Bç7…5§7ä4¸8­Dç7šA„4å5®EEæ6—7¶6è8¡1ŒCæ6ƒ3…5"2\C)/F);B
A 0 0assert.match(app,C 0/FresponseSymbolMismatch\C(quoteResult\C.value,C 0symbol,C 0"2å5®EEæ6—7¶6è8¡1ŒCæ6ƒ3…5"2\C)/F);B
A 0 0assert.match(app,C 0/FresponseSymbolMismatch\C(barsResult\C.value,C 0symbol,C 0"2æ6—7¥5 0K"2\C)/F);B
A 0 0assert.match(app,C 0/Fquote:A 0null,C[B\Cs\CS]D*?Fwarnings:A 0\C[B\C.\C.\C.\C(Array\C.isArray\C(bundleResponse\C.warnings\C)/F);B
A 0 0assert.ok(app.includes('return 0label 0+B 0"2è8¿F”4å5›BEç7šA„4è8¯F1å5ˆ8¸8ä4»B£3ç7 01 0"2 0+B 0responseSymbol 0+B 0"2 0ä4¸8Eè8¯F·7æ6±1‚2æ6 0‡7ç7šA„4 0"2 0+B 0requestedSymbol 0+B 0"2 0ä4¸8Då5ŒC¹9é9…5DïF¼CŒCå5·7²2å5¿F½Dç7•5¥5ç7»B“3æ6EœCã3€0‚2"2'));B
A}D);B
A
A 0
Atest("2å5ˆ8†6æ6E0ä4»B»Bå5ŠA¡1ã3€01é9˜8¶6æ6®Eµ5ä4ºA‹Bä4»B¶6ä4¸8E 0Run 0å5“3Då5ºA”4å5¿F…5é9¡1»Bå5ŒC¹9é9…5Då5½D“3å5‰9Dè8¯F·7æ6±1‚2æ6 0‡7è8¯F†6"2,C 0() 0=D>E 0{B
A 0 0assert.match(app,C 0/Ffunction 0responseIdentityError\C(message\C)[B\Cs\CS]D*?Ferror\C.responseIdentityMismatch 0=D 0true/F);B
A 0 0assert.match(app,C 0/Fcandidate\C.task_id 0!=D=D 0taskId[B\Cs\CS]D*?Fä4»B»Bå5ŠA¡1ç7ŠA¶6æ6€01æ6E¥5å5F£3æ6œCªAè8¿F”4å5›BE 0task_id/F);B
A 0 0assert.match(app,C 0/FeventPayload\C.task_id 0!=D=D 0taskId[B\Cs\CS]D*?Fé9˜8¶6æ6®Eµ5ä4ºA‹Bä4»B¶6æ6E¥5å5F£3æ6œCªAè8¿F”4å5›BE 0task_id/F);B
A 0 0assert.match(app,C 0/Fif 0\C(taskError 0&6&6 0\C(!isRetryableApiError\C(taskError\C) 0\C|C\C|C 0taskError\C.responseContractError\C)\C)/F);B
A 0 0assert.match(app,C 0/Fif 0\C(isRetryableApiError\C(eventError\C) 0&6&6 0!eventError\C.responseContractError\C)/F);B
A 0 0assert.match(app,C 0/Fif 0\C(result\C?F\C.run_id 0!=D=D 0runId\C) 0\C{B[B\Cs\CS]D*?FRun 0ç7»B“3æ6EœCæ6E¥5å5F£3æ6œCªAè8¿F”4å5›BE 0run_id/F);B
A 0 0assert.match(app,C 0/F!error\C.responseContractError 0&6&6 0isRetryableApiError\C(error\C)/F);B
A}D);B
A
A 0
Atest("2mobile 0navigation 0drawer 0moves 0focus 0inside 0and 0traps 0Tab 0navigation"2,C 0() 0=D>E 0{B
A 0 0assert.match(app,C 0/Fif 0\C(!wasOpen 0&6&6 0isOpen\C) 0\C$4\C("2\C.sidebar"2\C)\C.querySelector\C("2\C.nav-link:Anot\C(\C[Bdisabled\C]D\C)"2\C)\C?F\C.focus\C(\C)/F);B
A 0 0assert.match(app,C 0/Fif 0\C(event\C.key 0!=D=D 0"2Tab"2\C) 0return;B[B\Cs\CS]D*?Fif 0\C(window\C.innerWidth 0>E 0600\C) 0return;B[B\Cs\CS]D*?Fconst 0firstFocusable 0=D 0focusable\C[B0\C]D;B[B\Cs\CS]D*?Fconst 0lastFocusable 0=D 0focusable\C[Bfocusable\C.length 0- 01\C]D/F);B
A 0 0assert.match(app,C 0/Fevent\C.shiftKey[B\Cs\CS]D*?FlastFocusable\C.focus\C(\C)[B\Cs\CS]D*?FfirstFocusable\C.focus\C(\C)/F);B
A}D);B
Atest("2ç7§7»Bå5ŠA¨8ä4¾E§7æ6 0Fæ6‰9“3å5¼C€0è8¿FEæ6E¥5è8®E¾Eç7½D®Eæ6—7¶6å5…5³3é9—7­Dæ6ŠA½Då5±1‰9å5¹9¶6è8¿F›Bå5…5¥5è8®E¾Eç7½D®Eä4¸8­Då5¿Fƒ3"2,C 0() 0=D>E 0{B
A 0 0assert.match(html,C 0/Fclass=D"2nav-link"2[B^E>E]D*data-view=D"2settings"2/F);B
A 0 0assert.match(app,C 0/Fif 0\C(nextView 0=D=D=D 0"2settings"2 0&6&6 0previousView 0!=D=D 0nextView\C) 0openSettings\C(\C)/F);B
A 0 0assert.match(app,C 0/Fbutton\C.addEventListener\C("2click"2,C 0\C(\C) 0=D>E 0\C{B\Cs*setView\C(button\C.dataset\C.view\C);B\Cs*setMobileNavigationOpen\C(false,C 0\C{B 0restoreFocus:A 0true 0\C}D\C);B/F);B
A 0 0assert.doesNotMatch(app,C 0/Fsettings-dialog/F);B
A}D);B
Atest("2numeric 0UI 0presentation 0rejects 0boolean 0or 0structured 0market 0fields"2,C 0() 0=D>E 0{B
A 0 0assert.match(format,C 0/Fexport 0function 0finiteNumberOrNull\C(value\C)/F);B
A 0 0assert.match(format,C 0/Ftypeof 0value 0!=D=D 0"2number"2 0&6&6 0typeof 0value 0!=D=D 0"2string"2/F);B
A 0 0assert.match(marketOverview,C 0/Ffunction 0count\C(v\C) 0\C{B 0const 0n=DfiniteNumberOrNull\C(v\C)/F);B
A 0 0assert.match(marketOverview,C 0/Ffunction 0dir\C(v\C) 0\C{B 0const 0n=DfiniteNumberOrNull\C(v\C)/F);B
A 0 0assert.match(marketActivity,C 0/Ffunction 0amount\C(v\C)\C{Bconst 0n=DfiniteNumberOrNull\C(v\C)/F);B
A 0 0assert.match(stockResearch,C 0/Ffunction 0formatNumber\C(value\C)\C{Bconst 0number=DfiniteNumberOrNull\C(value\C)/F);B
A 0 0assert.match(app,C 0/Fconst 0changeValue 0=D 0finiteNumberOrNull\C(quote\C.change\C)/F);B
A 0 0assert.match(app,C 0/Fconst 0changePercentValue 0=D 0matched 0\C?F 0finiteNumberOrNull\C(quote\C.change_percent\C) 0:A 0null/F);B
A 0 0assert.match(app,C 0/Fconst 0ratio 0=D 0finiteNumberOrNull\C(v\C)/F);B
A 0 0assert.doesNotMatch(app,C 0/FNumber\C(quote\C.(?F:Achange|Cchange_percent|Cvolume_ratio)\C)|CNumber\C(bar\C.change_percent\C)/F,C 0"2market 0values 0must 0not 0use 0coercive 0Number() 0for 0display 0or 0direction 0styling"2);B
A 0 0assert.match(app,C 0/Fconst 0changePercent 0=D 0finiteNumberOrNull\C(bar\C.change_percent\C)/F);B
A}D);B
A
A 0
Atest("2ä4»B»Bå5ŠA¡1å5ˆ8›Bå5»BºAæ6ˆ80å5ŠAŸFå5“3Då5ºA”4ç7¼CºAå5°0‘1 0task_id 0æ6—7¶6æ6F0ç7¤4ºAç7»B“3æ6EœCæ6œCªAç7¡1®Eè8®E¤4ïF¼CŒCé91¿Få5…5Dè8¯F¯Fæ6ŠA¥5å5¤4±1è8´4¥5"2,C 0() 0=D>E 0{B
A 0 0assert.match(app,C 0/Fif 0\C(!task 0\C|C\C|C 0typeof 0task\C.task_id 0!=D=D 0"2string"2 0\C|C\C|C 0!task\C.task_id\C.trim\C(\C)\C) 0\C{B[B\Cs\CS]D*?Ferror\C.creationOutcomeUnknown 0=D 0true/F);B
A 0 0assert.match(app,C 0/Ftypeof 0task\C?F\C.task_id 0=D=D=D 0"2string"2 0&6&6 0task\C.task_id\C.trim\C(\C)/F);B
A 0 0assert.ok(app.includes("2error?F.creationOutcomeUnknown 0?F 0error.message"2));B
A 0 0assert.match(app,C 0/Fæ6—7 0æ6³3•5ç7¡1®Eè8®E¤4ä4»B»Bå5ŠA¡1æ6˜8¯Få50¦6å5·7²2å5ˆ8›Bå5»BºA[B\Cs\CS]D*?Fé91¿Få5…5Dç7›B²2ç7›B®Eé9‡7Då5¤4Dæ6F0ä4ºA¤4/F);B
A}D);B
A
A 0
Atest("2ä4»B»Bå5ŠA¡1ç7ŠA¶6æ6€01å5’2ŒCé9˜8¶6æ6®Eµ5ä4ºA‹Bä4»B¶6å5“3Då5ºA”4ç7»B“3æ6E„4æ6—7 0æ6•5ˆ8æ6—7¶6æ6šA‚2å51œCé9‡7Dè8¯F•5ïF¼CŒCæ6œCªAç7ŸF¥5ç7ŠA¶6æ6€01ä4¸8Dè8¿F›Bå5…5¥5è8½D®Eè8¯F¢2å5¾EªAç7E¯F"2,C 0() 0=D>E 0{B
A 0 0assert.match(app,C 0/Fconst 0TASK_STATUSES 0=D 0new 0Set\C(\C[B"2queued"2,C 0"2running"2,C 0"2succeeded"2,C 0"2partial"2,C 0"2failed"2\C]D\C)/F);B
A 0 0assert.match(app,C 0/Ffunction 0responseContractError\C(message\C)[B\Cs\CS]D*?Ferror\C.responseContractError 0=D 0true/F);B
A 0 0assert.match(app,C 0/Ftypeof 0candidate\C.status 0!=D=D 0"2string"2 0\C|C\C|C 0!TASK_STATUSES\C.has\C(candidate\C.status\C)/F);B
A 0 0assert.match(app,C 0/Ftypeof 0candidate\C.run_id 0!=D=D 0"2string"2 0\C|C\C|C 0!candidate\C.run_id\C.trim\C(\C)/F);B
A 0 0assert.match(app,C 0/Fstate\C.runId 0&6&6 0candidate\C.run_id 0!=D=D 0state\C.runId[B\Cs\CS]D*?Få5½D“3å5‰9Dä4»B»Bå5ŠA¡1å5·7²2å5…5³3è81”4ç7šA„4 0Run 0ä4¸8Dä4¸8€0è8‡7´4/F);B
A 0 0assert.match(app,C 0/Fä4»B»Bå5ŠA¡1ç7ŠA¶6æ6€01æ6E¥5å5F£3è8¿F”4å5›BEä4ºA†6æ6œCªAæ6”4¯Fæ6ŒC1ç7šA„4ç7ŠA¶6æ6€01/F);B
A 0 0assert.match(app,C 0/FtaskError 0=D 0responseContractError\C("2ä4»B»Bå5ŠA¡1ç7ŠA¶6æ6€01æ6E¥5å5F£3è8¿F”4å5›BEä4ºA†6æ6—7 0æ6³3•5è8¯F†6å5ˆ8«Bç7šA„4å5“3Då5ºA”4ç7»B“3æ6E„4ã3€0‚2"2\C)/F);B
A 0 0assert.match(app,C 0/FeventError 0=D 0responseContractError\C("2é9˜8¶6æ6®Eµ5ä4ºA‹Bä4»B¶6æ6E¥5å5F£3è8¿F”4å5›BEç7šA„4æ6•5°0æ6D®Eç7»B“3æ6E„4æ6—7 0æ6•5ˆ8ã3€0‚2"2\C)/F);B
A 0 0assert.match(app,C 0/Fif 0\C(!isRetryableApiError\C(eventError\C) 0\C|C\C|C 0eventError\C.responseContractError\C) 0state\C.taskEventsUnavailable 0=D 0true/F);B
A 0 0assert.match(app,C 0/Fif 0\C(delay 0!=D=D 0undefined 0&6&6 0!error\C.responseContractError 0&6&6 0isRetryableApiError\C(error\C)\C)/F);B
A}D);B
A
A 0
Atest("2Run 0ç7»B“3æ6EœCå5¿F…5é9¡1»Bä4¸8Eå5½D“3å5‰9Dä4»B»Bå5ŠA¡1æ6 0‡7ç7šA„4é9›B†6å50ˆ8ä4¸8€0è8‡7´4å50Eæ6‰9Då5…51è8®E¸8æ6¸8²2æ6ŸF“3"2,C 0() 0=D>E 0{B
A 0 0assert.match(app,C 0/Ffunction 0runResultSymbolMismatch\C(result\C)/F);B
A 0 0assert.match(app,C 0/Fconst 0expectedRaw 0=D 0Array\C.isArray\C(state\C.taskSymbols\C) 0\C?F 0state\C.taskSymbols 0:A 0\C[B\C]D/F);B
A 0 0assert.match(app,C 0/FRun 0ç7»B“3æ6EœCæ6œCªAè8¿F”4å5›BEè8¯F·7æ6±1‚2æ6 0‡7ç7šA„4å5ˆ8—7è8¡1¨8ïF¼CŒCæ6—7 0æ6³3•5ç7¡1®Eè8®E¤4æ6˜8¯Få50¦6å5±1Eä4ºAEå5½D“3å5‰9Dä4»B»Bå5ŠA¡1ïF¼CŒCå5·7²2å5¿F½Dç7•5¥5/F);B
A 0 0assert.match(app,C 0/Ffunction 0sameTaskSymbolSet\C(expectedRaw,C 0actualRaw\C)[B\Cs\CS]D*?FnormalizedTaskSymbolSet\C(expectedRaw\C)[B\Cs\CS]D*?FnormalizedTaskSymbolSet\C(actualRaw\C)[B\Cs\CS]D*?FJSON\C.stringify\C(expected\C) 0=D=D=D 0JSON\C.stringify\C(actual\C)/F);B
A 0 0assert.match(app,C 0/Fconst 0symbolMismatch 0=D 0runResultSymbolMismatch\C(result\C);B[B\Cs\CS]D*?Fif 0\C(symbolMismatch\C) 0throw 0responseIdentityError\C(symbolMismatch\C);B[B\Cs\CS]D*?FrenderTaskResult\C(result\C)/F);B
A 0 0assert.match(app,C 0/Fgeneration 0!=D=D 0state\C.runResultGeneration 0\C|C\C|C 0state\C.taskId 0!=D=D 0expectedTaskId 0\C|C\C|C\Cs*endpointGeneration 0!=D=D 0state\C.analysisConnectionGeneration 0\C|C\C|C\Cs*state\C.taskAnalysisBase 0!=D=D 0analysisBase/F);B
A}D);B
A
A 0
Atest("2ä4»B»Bå5ŠA¡1ç7ŠA¶6æ6€01è8½D®Eè8¯F¢2ä4¸8Då5…51è8®E¸8æ6›B´4æ6D¢2å5·7²2å5»BºAç7«B‹Bç7šA„4 0task_id/Frun_id 0å5…5³3è81”4"2,C 0() 0=D>E 0{B
A 0 0const 0poller 0=D 0app.match(/Fasync 0function 0pollTask\C([B\Cs\CS]D*?F\C\Cn\C}D/F)?F.[B0]D 0|C|C 0"2"2;B
A 0 0assert.ok(poller,C 0"2task 0poller 0must 0exist"2);B
A 0 0assert.match(poller,C 0/Fcandidate\C.task_id 0!=D=D 0taskId/F);B
A 0 0assert.match(poller,C 0/Ftypeof 0candidate\C.run_id 0!=D=D 0"2string"2 0\C|C\C|C 0!candidate\C.run_id\C.trim\C(\C)/F);B
A 0 0assert.match(poller,C 0/Fstate\C.runId 0&6&6 0candidate\C.run_id 0!=D=D 0state\C.runId/F);B
A 0 0assert.match(poller,C 0/Fä4»B»Bå5ŠA¡1ç7ŠA¶6æ6€01æ6E¥5å5F£3è8¿F”4å5›BEç7šA„4 0run_id 0ä4¸8Eå5½D“3å5‰9Dä4»B»Bå5ŠA¡1å5·7²2å5…5³3è81”4ç7šA„4 0Run 0ä4¸8Dä4¸8€0è8‡7´4/F);B
A 0 0assert.match(poller,C 0/FtaskError 0=D 0responseIdentityError\C([B\Cs\CS]D*?Frun_id 0ä4¸8Eå5½D“3å5‰9Dä4»B»Bå5ŠA¡1å5·7²2å5…5³3è81”4ç7šA„4 0Run 0ä4¸8Dä4¸8€0è8‡7´4/F);B
A}D);B
A
A 0
Atest("2ä4¸8ªAè8‚2¡1è8¡1ŒCæ6ƒ3…5ã3€01ç7 0”4ç7©9¶6æ6‘1˜8è8¦61ä4¸8Eå5D‡7ç7ºA¿Få5ŒC¹9é9…5Dæ6—7¶6å5…5¼Cå5®E¹9ç7ºA¯Få5…5­Dä4½DDä4»B£3ç7 01å5’2ŒCå5¸8¦6ä4ºA¤4æ6˜8“3æ6‰9€0å50Eç7¼C€0ä4»B£3ç7 01"2,C 0() 0=D>E 0{B
A 0 0assert.match(app,C 0/Ffunction 0sameMarketSymbol\C(left,C 0right\C)/F);B
A 0 0assert.match(app,C 0/Fitems\C.some\C(\C(item\C) 0=D>E 0sameMarketSymbol\C(item\C.symbol,C 0currentSymbol\C)\C)/F);B
A 0 0assert.match(app,C 0/FtechnicalSymbol 0&6&6 0sameMarketSymbol\C(technicalSymbol,C 0currentSymbol\C)/F);B
A 0 0assert.match(app,C 0/F!sameMarketSymbol\C(technicalSymbol,C 0currentSymbol\C)/F);B
A 0 0assert.match(app,C 0/FsameMarketSymbol\C(item\C.symbol,C 0currentSymbol\C) 0&6&6 0Boolean\C(quote\C)/F);B
A 0 0assert.match(app,C 0/Fstate\C.quote 0&6&6 0sameMarketSymbol\C(state\C.quote\C.symbol 0\C|C\C|C 0"2"2,C 0symbol\C)/F);B
A}D);B
A
A 0
Atest("2è8‡7ªAé9€0‰9è8‚2¡1æ6ŠA¥5ä4»B·7å5’2ŒCä4»B»Bå5ŠA¡1å5¼C•5ç7”4¨8ç7šA„4è8‚2¡1ç7¥5¨8ä4»B£3ç7 01æ6¯F”4è8¾Eƒ3å5…5¼Cå5®E¹9ä4ºA¤4æ6˜8“3æ6‰9€0å50Eç7¼C€0å5·7®Eå5¼C‚2"2,C 0() 0=D>E 0{B
A 0 0assert.match(app,C 0/Ffunction 0normalizeWatchlistQuoteSymbol\C(value\C) 0\C{B\Cs*try 0\C{B 0return 0comparableMarketSymbol\C(value\C);B/F);B
A 0 0assert.match(app,C 0/Fconst 0normalizeLocalSymbol 0=D 0normalizeWatchlistQuoteSymbol/F);B
A 0 0assert.match(app,C 0/Fconst 0quoteMatches 0=D 0Boolean\C(state\C.quote\C?F\C.symbol\C) 0&6&6 0normalizeLocalSymbol\C(state\C.quote\C.symbol\C) 0=D=D=D 0itemSymbol/F);B
A 0 0assert.match(app,C 0/Ftask\C.symbols\C.some\C(\C(value\C) 0=D>E 0normalizeLocalSymbol\C(value\C) 0=D=D=D 0itemSymbol\C)/F);B
A 0 0assert.match(app,C 0/Fstate\C.taskSymbols\C.some\C(\C(value\C) 0=D>E 0normalizeLocalSymbol\C(value\C) 0=D=D=D 0itemSymbol\C)/F);B
A 0 0assert.match(app,C 0/Fstate\C.quote\C?F\C.name 0&6&6 0state\C.quote\C?F\C.symbol 0&6&6 0sameMarketSymbol\C(state\C.quote\C.symbol,C 0symbol\C)/F);B
A}D);B
A
A 0
Atest("2é9˜8¶6æ6®Eµ5ä4ºA‹Bä4»B¶6ä4¸8¥5æ6 0¼Cæ6 0¡1é9ªAŒCæ6•5°0å5€0¼Cå5ºAFå5F·7ä4¸8E 0event 0å5¯F¹9è8±1¡1ç7»B“3æ6E„4"2,C 0() 0=D>E 0{B
A 0 0assert.match(app,C 0/Fconst 0sequence 0=D 0typeof 0item\C?F\C.sequence 0=D=D=D 0"2number"2 0\C?F 0item\C.sequence 0:A 0NaN/F);B
A 0 0assert.match(app,C 0/F!Number\C.isSafeInteger\C(sequence\C) 0\C|C\C|C 0sequence 0<C 01/F);B
A 0 0assert.match(app,C 0/F!item\C.event 0\C|C\C|C 0typeof 0item\C.event 0!=D=D 0"2object"2 0\C|C\C|C 0Array\C.isArray\C(item\C.event\C)/F);B
A 0 0assert.match(app,C 0/Få5­D—7æ6®Eµ5æ6—7 0æ6•5ˆ8ç7šA„4é9˜8¶6æ6®Eµ5ä4ºA‹Bä4»B¶6ïF¼Cˆ8å5ºAFå5F·7æ6ˆ8–6 0event 0ç7»B“3æ6E„4ä4¸8Dç7¬C¦6å50ˆ8å5¥5‘1ç7ºA¦6ïF¼C‰9ïF¼CŒCå5·7²2å5¿F½Dç7•5¥5/F);B
A 0 0assert.doesNotMatch(app,C 0/Fconst 0sequence 0=D 0Number\C(item\C?F\C.sequence\C)/F,C 0"2boolean 0sequence 0must 0not 0coerce 0to 0integer 01"2);B
A}D);B
A
A 0
Atest("2é9˜8¶6æ6®Eµ5ä4ºA‹Bä4»B¶6ç7šA„4 0run_id 0å5¿F…5é9¡1»Bå5ŒC¹9é9…5Då5½D“3å5‰9Dä4»B»Bå5ŠA¡1å5…5³3è81”4ç7šA„4 0Run"2,C 0() 0=D>E 0{B
A 0 0assert.match(app,C 0/Fconst 0expectedEventRunId 0=D 0state\C.runId 0\C|C\C|C 0task\C?F\C.run_id 0\C|C\C|C 0"2"2/F);B
A 0 0assert.match(app,C 0/Fconst 0eventRunId 0=D 0typeof 0item\C.event\C.run_id 0=D=D=D 0"2string"2 0\C?F 0item\C.event\C.run_id\C.trim\C(\C) 0:A 0"2"2/F);B
A 0 0assert.match(app,C 0/F!expectedEventRunId 0\C|C\C|C 0!eventRunId 0\C|C\C|C 0eventRunId 0!=D=D 0expectedEventRunId/F);B
A 0 0assert.match(app,C 0/FRun 0ID 0ç7¼CºAå5¤4±1æ6ˆ8–6ä4¸8Eå5½D“3å5‰9Dä4»B»Bå5ŠA¡1å5…5³3è81”4ç7šA„4 0Run 0ä4¸8Dä4¸8€0è8‡7´4ïF¼CŒCå5·7²2å5¿F½Dç7•5¥5/F);B
A 0 0assert.match(app,C 0/Få5­D—7æ6®Eµ5æ6—7 0æ6•5ˆ8ç7šA„4é9˜8¶6æ6®Eµ5ä4ºA‹Bä4»B¶6ïF¼Cˆ8å5ºAFå5F·7æ6ˆ8–6 0event 0ç7»B“3æ6E„4ä4¸8Dç7¬C¦6å50ˆ8å5¥5‘1ç7ºA¦6ïF¼C‰9ïF¼CŒCå5·7²2å5¿F½Dç7•5¥5/F);B
A}D);B
A
Atest("2å5¢2Eé9‡7Fé9˜8¶6æ6®Eµ5ä4ºA‹Bä4»B¶6ä4¸8Dä4¼CšAè8¦6†6ç7›B–6å5·7²2è8¯F»Bå5F–6ç7šA„4ç7›B¸8å50ŒCæ6ˆ8–6æ6›B´4æ6—7§7å5ºAFå5F·7"2,C 0() 0=D>E 0{B
A 0 0assert.match(app,C 0/Fconst 0previousSequence 0=D 0state\C.taskSequence/F);B
A 0 0assert.match(app,C 0/Fsequence 0<C=D 0previousSequence 0\C|C\C|C 0state\C.taskEvents\C.has\C(sequence\C)/F);B
A 0 0assert.match(app,C 0/Fstate\C.taskEvents\C.set\C(sequence,C 0normalized\C)/F);B
A 0 0assert.match(app,C 0/Fé9‡7Då5¤4Dæ6ˆ8–6æ6—7§7å5ºAFå5F·7ç7šA„4é9˜8¶6æ6®Eµ5ä4ºA‹Bä4»B¶6ïF¼CŒCå5·7²2å5¿F½Dç7•5¥5/F);B
A}D);B
A
Atest("2ç7»Bˆ8æ6€01ä4»B»Bå5ŠA¡1æ6ŒC‰9 0next_after_sequence 0æ6E’2ç7©9ºAé9˜8¶6æ6®Eµ5ä4ºA‹Bä4»B¶6å5ˆ8†6é9¡1µ5å50Eæ6‰9Dè8¯F»Bå5F–6 0Run 0æ6ŠA¥5å5‘1ŠA"2,C 0() 0=D>E 0{B
A 0 0assert.match(app,C 0/FtaskEventCursor:A 00/F);B
A 0 0const 0pollStart 0=D 0app.indexOf("2async 0function 0pollTask("2);B
A 0 0const 0pollEnd 0=D 0app.indexOf("2\C\Cnasync 0function 0createTask("2,C 0pollStart);B
A 0 0const 0poller 0=D 0pollStart 0>E=D 00 0&6&6 0pollEnd 0>E 0pollStart 0?F 0app.slice(pollStart,C 0pollEnd) 0:A 0"2"2;B
A 0 0assert.match(poller,C 0/Fconst 0requestedEventCursor 0=D 0state\C.taskEventCursor/F);B
A 0 0assert.match(poller,C 0/FgetAnalysisTaskEvents\C(requestSettings,C 0taskId,C 0requestedEventCursor\C)/F);B
A 0 0assert.match(poller,C 0/F!Number\C.isSafeInteger\C(nextAfterSequence\C)/F);B
A 0 0assert.match(poller,C 0/Fstate\C.taskEventCursor 0=D 0nextAfterSequence/F);B
A 0 0assert.match(poller,C 0/FeventPageHasMore 0=D 0eventPayload\C.events\C.length 0>E=D 0TASK_EVENT_PAGE_SIZE 0&6&6[B\Cs\CS]D*?FnextAfterSequence 0>E 0requestedEventCursor/F);B
A 0 0assert.match(poller,C 0/Fif 0\C(task 0&6&6 0TERMINAL_TASK_STATUSES\C.has\C(task\C.status\C)\C) 0\C{B[B\Cs\CS]D*?Fif 0\C(eventPageHasMore 0&6&6 0!eventError 0&6&6 0!eventsSkipped\C) 0\C{B[B\Cs\CS]D*?Fæ6­D£3å5œC¨8ç7»B§7ç7»B­Dè8¯F»Bå5F–6å5‰9©9ä4½D™9é9˜8¶6æ6®Eµ5ä4ºA‹Bä4»B¶6[B\Cs\CS]D*?FsetTimeout\C(\C(\C) 0=D>E 0pollTaskSafely\C(generation\C),C 00\C)[B\Cs\CS]D*?Freturn;B[B\Cs\CS]D*?FstopTaskPolling\C(\C)[B\Cs\CS]D*?FloadTaskRunResult/F);B
A 0 0assert.match(poller,C 0/Fif 0\C(eventError 0&6&6 0isRetryableApiError\C(eventError\C) 0&6&6 0!eventError\C.responseContractError\C) 0\C{B[B\Cs\CS]D*?F5 0ç7§7’2å50Eè8‡7ªAå5ŠA¨8é9‡7Dè8¯F•5[B\Cs\CS]D*?FsetTimeout\C(\C(\C) 0=D>E 0pollTaskSafely\C(generation\C),C 05000\C)/F);B
A 0 0const 0refreshStart 0=D 0app.indexOf('$4("2#3refresh-task"2).addEventListener("2click"2');B
A 0 0const 0refreshEnd 0=D 0app.indexOf('$4("2#3recent-analysis-search"2)',C 0refreshStart);B
A 0 0const 0refreshHandler 0=D 0refreshStart 0>E=D 00 0&6&6 0refreshEnd 0>E 0refreshStart 0?F 0app.slice(refreshStart,C 0refreshEnd) 0:A 0"2"2;B
A 0 0assert.ok(refreshHandler.includes("2state.taskEventsUnavailable 0=D 0false"2),C 0"2æ6‰9‹Bå5ŠA¨8å5ˆ8·7æ6–6°0å5¿F…5é9¡1»Bæ61¢2å5¤4Dä4ºA‹Bä4»B¶6è8¯F»Bå5F–6"2);B
A 0 0assert.ok(refreshHandler.includes("2pollTaskSafely(generation)"2),C 0"2æ6‰9‹Bå5ŠA¨8å5ˆ8·7æ6–6°0å5¿F…5é9¡1»Bç7«B‹Bå5D³3é9‡7Dæ6–6°0å5F‘1è8µ5·7 0Task/Fä4ºA‹Bä4»B¶6è8¯F»Bå5F–6"2);B
A 0 0assert.doesNotMatch(refreshHandler,C 0/Fstate\C.taskEventCursor\Cs*=D\Cs*0/F,C 0"2æ6‰9‹Bå5ŠA¨8é9‡7Dè8¯F•5å5ºA”4ä4¿FDç7•5™9å5·7²2æ6E¨8è8¿F›Bç7šA„4æ6œCDå5ŠA¡1ç7«B¯Fæ6¸8¸8æ6 0‡7ïF¼CŒCé91¿Få5…5Dé9‡7Dæ6–6°0è8¯F»Bå5F–6ç7¬C¬Cä4¸8€0é9¡1µ5"2);B
A 0 0assert.equal((app.match(/Fstate\C.taskSequence 0=D 00;B\Cs*state\C.taskEventCursor 0=D 00;B/Fg) 0|C|C 0[B]D).length,C 03,C 0"2æ6–6°0å5»BºAã3€01æ61¢2å5¤4Då5’2ŒCæ6¸8…5ç7©9ºAä4»B»Bå5ŠA¡1æ6—7¶6é9ƒ3½Dé9‡7Dç7½D®Eç7‹B¬Cç7«B‹Bä4ºA‹Bä4»B¶6æ6¸8¸8æ6 0‡7"2);B
A}D);B
A
Atest("2å5ˆ8†6æ6E0 0API 0å5ˆ8‡7æ6D¢2æ6œCŸFé9—7´4ç7šA„4 0UI 0æ6¸8…5ç70†6å5¼C‚2å5¸8¸8ä4¸8Dä4¼CšAä4¸8­Dæ6–6­Dé9…5Dç7½D®Eå5ºA”4ç7”4¨8"2,C 0() 0=D>E 0{B
A 0 0const 0helper 0=D 0app.match(/Ffunction 0safelyUpdateTaskContextUi\C(label,C 0update\C) 0\C{B[B\Cs\CS]D*?F\C\Cn\C}D/F)?F.[B0]D 0|C|C 0"2"2;B
A 0 0assert.match(helper,C 0/Ftry 0\C{B\Cs*update\C(\C);B\Cs*\C}D 0catch 0\C(error\C)/F);B
A 0 0assert.match(helper,C 0/Fé9ƒ3¨8å5ˆ8†6ç7•5ŒCé9D¢2æ6¸8…5ç70†6å5¤4±1è8´4¥5[B\Cs\CS]D*?Fcatch/F);B
A
A 0 0const 0invalidateStart 0=D 0app.indexOf("2function 0invalidateActiveTaskForAnalysisBaseChange("2);B
A 0 0const 0invalidateEnd 0=D 0app.indexOf("2\C\Cnfunction 0applySettings("2,C 0invalidateStart);B
A 0 0const 0invalidate 0=D 0invalidateStart 0>E=D 00 0&6&6 0invalidateEnd 0>E 0invalidateStart
A 0 0 0 0?F 0app.slice(invalidateStart,C 0invalidateEnd) 0:A 0"2"2;B
A 0 0assert.match(invalidate,C 0/Fstate\C.taskId 0=D 0"2"2;B[B\Cs\CS]D*?FsafelyUpdateTaskContextUi\C("2æ6¸8…5ç70†6ä4»B»Bå5ŠA¡1è8¯F¦6æ6ƒ3…5å5¤4±1è8´4¥5ïF¼CšA[B\Cs\CS]D*?FsafelyUpdateTaskContextUi\C("2é9‡7Dç7»B˜8é9˜8¶6æ6®Eµ5ä4ºA‹Bä4»B¶6å5¤4±1è8´4¥5ïF¼CšA[B\Cs\CS]D*?FsafelyUpdateTaskContextUi\C("2é9‡7Dç7»B˜8è8‡7ªAé9€0‰9è8‚2¡1å5D¡1ç7‰9‡7å5¤4±1è8´4¥5ïF¼CšA/F);B
A 0 0assert.doesNotMatch(invalidate,C 0/F(^E|C
A)\Cs*renderTaskEvents\C(\C);B|C(^E|C
A)\Cs*renderTaskFlowSummary\C(\C);B|C(^E|C
A)\Cs*renderRecentTasks\C(\C);B/F);B
A
A 0 0const 0applyStart 0=D 0app.indexOf("2function 0applySettings("2);B
A 0 0const 0applyEnd 0=D 0app.indexOf("2\C\Cnfunction 0saveSettingsFromForm("2,C 0applyStart);B
A 0 0const 0apply 0=D 0applyStart 0>E=D 00 0&6&6 0applyEnd 0>E 0applyStart 0?F 0app.slice(applyStart,C 0applyEnd) 0:A 0"2"2;B
A 0 0assert.match(apply,C 0/Fif 0\C(analysisBaseChanged\C) 0\C{B[B\Cs\CS]D*?FrunSettingsApplyStep\C("2æ6¸8…5ç70†6æ6—7§7å5ˆ8†6æ6E0ä4»B»Bå5ŠA¡1å5¤4±1è8´4¥5ïF¼CšA"2,C 0\C(\C) 0=D>E\Cs*invalidateActiveTaskForAnalysisBaseChange\C(previousSettings\C.analysisBase,C 0nextSettings\C.analysisBase\C),C 0failures\C);B/F);B
A 0 0assert.match(apply,C 0/Fif 0\C(marketConnectionChanged\C) 0\C{B[B\Cs\CS]D*?FrunSettingsApplyStep\C("2å5¤4±1æ6•5ˆ8å5¸8‚2å5œCºAæ6€0»Bè8§7ˆ8è8¯F·7æ6±1‚2å5¤4±1è8´4¥5ïF¼CšA"2,C 0\C(\C) 0=D>E 0marketOverview\C.invalidateConnection\C(\C),C 0failures\C);B[B\Cs\CS]D*?FrunSettingsApplyStep\C("2å5¤4±1æ6•5ˆ8å5¸8‚2å5œCºAæ6´4»Bå5ŠA¨8è8¯F·7æ6±1‚2å5¤4±1è8´4¥5ïF¼CšA"2,C 0\C(\C) 0=D>E 0marketActivity\C.invalidateConnection\C(\C),C 0failures\C);B/F);B
A 0 0assert.match(apply,C 0/Fif 0\C(analysisBaseChanged 0\C|C\C|C 0marketConnectionChanged\C) 0\C{B[B\Cs\CS]D*?FrunSettingsApplyStep\C("2å5¤4±1æ6•5ˆ8æ6•5°0æ6D®Eä4¸8­Då5¿Fƒ3æ6£3€0æ6ŸF¥5ç7»B“3æ6EœCå5¤4±1è8´4¥5ïF¼CšA"2,C 0\C(\C) 0=D>E\Cs*dataCenter\C.invalidate\C(/F);B
A 0 0assert.match(apply,C 0/FrunSettingsApplyStep\C("2é9‡7Dç7½D®Eè8¿FEæ6E¥5é9…5Dç7½D®Eè8¡1¨8å5D•5å5¤4±1è8´4¥5ïF¼CšA"2,C 0\C(\C) 0=D>E 0\C{B/F);B
A 0 0assert.match(apply,C 0/FinitialConnectionDraft 0=D 0null;B[B\Cs\CS]D*?FsyncConnectionDraftState\C(\C)/F);B
A}D);B
A
Atest("2ä4»B»Bå5ŠA¡1å5ˆ8‡7æ6D¢2å5’2ŒCå5ˆ8†6æ6E0è8¿FEæ6E¥5å5ˆ8‡7æ6D¢2ä4¼CšAé9‡7Dç7½D®Eä4ºA‹Bä4»B¶6æ6¸8¸8æ6 0‡7å5¹9¶6é9šA”4ç7¦6»Bè8¿FŸFå5ˆ8°0è8½D®Eè8¯F¢2"2,C 0() 0=D>E 0{B
A 0 0const 0contexts 0=D 0[B
A 0 0 0 0[B"2new 0task 0creation"2,C 0"2async 0function 0createTask(event) 0{B"2,C 0"2function 0startAnalysisForSymbol("2]D,C
A 0 0 0 0[B"2recent 0task 0restoration"2,C 0"2function 0activateTaskReference(record) 0{B"2,C 0"2function 0restoreLastTask("2]D,C
A 0 0 0 0[B"2analysis 0API 0change"2,C 0"2function 0invalidateActiveTaskForAnalysisBaseChange("2,C 0"2function 0applySettings("2]D
A 0 0]D;B
A 0 0for 0(const 0[Blabel,C 0startMarker,C 0endMarker]D 0of 0contexts) 0{B
A 0 0 0 0const 0start 0=D 0app.indexOf(startMarker);B
A 0 0 0 0const 0end 0=D 0app.indexOf(endMarker,C 0start 0+B 0startMarker.length);B
A 0 0 0 0assert.ok(start 0>E=D 00 0&6&6 0end 0>E 0start,C 0label 0+B 0"2 0state-reset 0function 0must 0exist"2);B
A 0 0 0 0const 0body 0=D 0app.slice(start,C 0end);B
A 0 0 0 0assert.match(body,C 0/Fstate\C.taskSequence 0=D 00;B\Cs*state\C.taskEventCursor 0=D 00;B\Cs*state\C.taskEvents\C.clear\C(\C)/F,C 0label 0+B 0"2 0resets 0both 0event 0cursors 0and 0stored 0events"2);B
A 0 0}D
A 0 0assert.match(app,C 0/Ffunction 0stopTaskPolling\C(\C) 0\C{B[B\Cs\CS]D*?Fstate\C.pollGeneration 0\C+B=D 01/F);B
A 0 0assert.match(app,C 0/Fif 0\C(generation 0!=D=D 0state\C.pollGeneration 0\C|C\C|C 0taskId 0!=D=D 0state\C.taskId 0\C|C\C|C 0endpointGeneration 0!=D=D 0state\C.analysisConnectionGeneration 0\C|C\C|C 0state\C.taskAnalysisBase 0!=D=D 0analysisBase 0\C|C\C|C 0state\C.settings\C.analysisBase 0!=D=D 0analysisBase\C) 0return;B/F);B
A}D);B
A
A 0
Atest("2ä4»B»Bå5ŠA¡1ç7ŠA¶6æ6€01å5›BEå5ŒC…5ä4¸8Dè8ƒ3½Dæ6›B¿Fæ6D¢2ä4»B»Bå5ŠA¡1æ6 0‡7ç7šA„4é9›B†6å50ˆ8ïF¼CŒCRun 0ä4¸8Eä4»B»Bå5ŠA¡1å5¤4Dç7”4¨8å50ŒCä4¸8€0è8§7„4è8ŒCƒ3å5ŒC–6æ6¯F”4è8¾Eƒ3"2,C 0() 0=D>E 0{B
A 0 0assert.match(app,C 0/Ffunction 0sameTaskSymbolSet\C(expectedRaw,C 0actualRaw\C)/F);B
A 0 0assert.match(app,C 0/Fcandidate\C.symbols 0!=D=D 0undefined 0&6&6 0!Array\C.isArray\C(candidate\C.symbols\C)/F);B
A 0 0assert.match(app,C 0/FArray\C.isArray\C(candidate\C.symbols\C) 0&6&6 0state\C.taskSymbols\C.length 0&6&6[B\Cs\CS]D*?F!sameTaskSymbolSet\C(state\C.taskSymbols,C 0candidate\C.symbols\C)/F);B
A 0 0assert.match(app,C 0/Fä4»B»Bå5ŠA¡1ç7ŠA¶6æ6€01æ6E¥5å5F£3è8¿F”4å5›BEç7šA„4æ6 0‡7ç7šA„4å5ˆ8—7è8¡1¨8ä4¸8Eå5½D“3å5‰9Dä4»B»Bå5ŠA¡1ä4¸8Dä4¸8€0è8‡7´4ïF¼CŒCå5·7²2å5¿F½Dç7•5¥5/F);B
A 0 0assert.match(app,C 0/Ffunction 0runResultSymbolMismatch\C(result\C)[B\Cs\CS]D*?FsameTaskSymbolSet\C(expectedRaw,C 0actualRaw\C)/F);B
A}D);B
Atest("2switching 0task 0context 0or 0analysis 0API 0endpoint 0resets 0the 0stage-event 0filter"2,C 0() 0=D>E 0{B
A 0 0const 0contexts 0=D 0[B
A 0 0 0 0[B"2recent 0task 0activation"2,C 0"2function 0activateTaskReference(record) 0{B"2,C 0"2function 0restoreLastTask("2]D,C
A 0 0 0 0[B"2task 0creation"2,C 0"2async 0function 0createTask(event) 0{B"2,C 0"2function 0startAnalysisForSymbol("2]D,C
A 0 0 0 0[B"2analysis 0API 0endpoint 0change"2,C 0"2function 0invalidateActiveTaskForAnalysisBaseChange("2,C 0"2function 0applySettings("2]D
A 0 0]D;B
A 0 0for 0(const 0[Blabel,C 0startMarker,C 0endMarker]D 0of 0contexts) 0{B
A 0 0 0 0const 0start 0=D 0app.indexOf(startMarker);B
A 0 0 0 0const 0end 0=D 0app.indexOf(endMarker,C 0start 0+B 0startMarker.length);B
A 0 0 0 0assert.ok(start 0>E=D 00 0&6&6 0end 0>E 0start,C 0label 0+B 0"2 0state-reset 0function 0must 0exist"2);B
A 0 0 0 0const 0body 0=D 0app.slice(start,C 0end);B
A 0 0 0 0assert.match(
A 0 0 0 0 0 0body,C
A 0 0 0 0 0 0/Fstate\C.taskEvents\C.clear\C(\C);B\Cs*state\C.taskEventFilter 0=D 0"2all"2;B\Cs*state\C.taskEventsLoaded 0=D 0false;B/F,C
A 0 0 0 0 0 0label 0+B 0"2 0must 0clear 0prior 0task 0events 0and 0restore 0the 0default 0filter"2
A 0 0 0 0);B
A 0 0}D
A}D);B
A
A
Atest("2å5·7¥5ä4½DœCå5ŒCºAå5¯F¼Cå5…5¥5ä4¸8Eè8‡7ªAé9€0‰9è8‚2¡1å5­D˜8å5‚2¨8ä4½D¿Fç7”4¨8å50ŒCä4¸8€0è8§7„4å5ˆ8™9è8¯F†6å5ˆ8«Bå5¸8¦6å50Eç7¼C€0ç7šA„4é9‡7Då5¤4Dè8¯F1å5ˆ8¸8"2,C 0() 0=D>E 0{B
A 0 0assert.match(format,C 0/Fexport 0function 0comparableMarketSymbol\C(input\C)/F);B
A 0 0const 0importBody 0=D 0app.match(/Ffunction 0validateBrowserDataPayload\C(payload\C) 0\C{B[B\Cs\CS]D*?F\C\Cn\C}D/F)?F.[B0]D 0|C|C 0"2"2;B
A 0 0assert.ok(importBody,C 0"2å5·7¥5ä4½DœCå5ŒCºAæ6•5°0æ6D®Eå5¯F¼Cå5…5¥5å5¿F…5é9¡1»Bå5…5ˆ8å5®EŒCæ6ˆ80ç7»B“3æ6E„4æ6 0¡1é9ªAŒC"2);B
A 0 0assert.match(importBody,C 0/Fconst 0symbolIdentity 0=D 0comparableMarketSymbol\C(symbol\C)/F);B
A 0 0assert.match(importBody,C 0/FseenSymbols\C.has\C(symbolIdentity\C)/F);B
A 0 0assert.match(importBody,C 0/Fè8‡7ªAé9€0‰9è8‚2¡1æ6–6‡7ä4»B¶6å5ŒC…5å50«Bé9‡7Då5¤4Dè8¯F1å5ˆ8¸8/F);B
A 0 0assert.match(watchlistModule,C 0/Fimport 0\C{B 0comparableMarketSymbol 0\C}D 0from 0"2\C.\C/Fformat\C.js"2/F);B
A 0 0assert.match(watchlistModule,C 0/Fconst 0identity 0=D 0comparableMarketSymbol\C(normalized\C.symbol\C)/F);B
A 0 0assert.match(watchlistModule,C 0/Fseen\C.has\C(identity\C)/F);B
A}D);B
A
A
Atest("2æ6¸8…5ç7©9ºAæ6ˆ8–6å5¯F¼Cå5…5¥5æ6œC¬Cå5œC°0ä4»B»Bå5ŠA¡1å5E†6å5F²2ä4¼CšAé9šA”4ç7¦6»Bå5œC¨8é9€0”4å5ˆ8›Bå5»BºAè8¯F·7æ6±1‚2ç7šA„4æ6™9šAå5ˆ8°0å5“3Då5ºA”4"2,C 0() 0=D>E 0{B
A 0 0assert.match(app,C 0/FtaskHistoryGeneration:A 00/F);B
A 0 0const 0createStart 0=D 0app.indexOf("2async 0function 0createTask(event) 0{B"2);B
A 0 0const 0createEnd 0=D 0app.indexOf("2function 0startAnalysisForSymbol("2,C 0createStart 0+B 01);B
A 0 0const 0creator 0=D 0createStart 0>E=D 00 0&6&6 0createEnd 0>E 0createStart 0?F 0app.slice(createStart,C 0createEnd) 0:A 0"2"2;B
A 0 0assert.ok(creator,C 0"2ä4»B»Bå5ŠA¡1å5ˆ8›Bå5»BºAæ6–6¹9æ6³3•5å5¿F…5é9¡1»Bå5­D˜8å5œC¨8"2);B
A 0 0assert.match(creator,C 0/FcreationHistoryGeneration 0=D 0state\C.taskHistoryGeneration;B/F);B
A 0 0assert.match(creator,C 0/Fconst 0taskHistoryWasReplaced 0=D 0creationHistoryGeneration 0!=D=D 0state\C.taskHistoryGeneration;B/F);B
A 0 0assert.match(creator,C 0/Fif 0\C(taskHistoryWasReplaced\C) 0\C{B[B\Cs\CS]D*?Fæ6œCªAé9‡7Dæ6–6°0å5†6™9å5…5¥5è8¯F¥5å5¼C•5ç7”4¨8[B\Cs\CS]D*?F\C}D 0else 0\C{B[B\Cs\CS]D*?FpersistRecentTaskHistory\C(endpointRecord\C)/F);B
A 0 0const 0clearStart 0=D 0app.indexOf("2function 0clearRecentTaskHistory() 0{B"2);B
A 0 0const 0clearEnd 0=D 0app.indexOf("2function 0setWatchlistFeedback("2,C 0clearStart 0+B 01);B
A 0 0const 0clearBody 0=D 0clearStart 0>E=D 00 0&6&6 0clearEnd 0>E 0clearStart 0?F 0app.slice(clearStart,C 0clearEnd) 0:A 0"2"2;B
A 0 0assert.match(clearBody,C 0/Fstate\C.taskHistoryGeneration 0\C+B=D 01;B\Cs*state\C.taskContextGeneration 0\C+B=D 01;B/F);B
A 0 0const 0importBody 0=D 0app.match(/Fasync 0function 0applyBrowserDataImport\C(imported,C 0currentWatchlist,C 0currentRecentTasks,C 0strategy\C) 0\C{B[B\Cs\CS]D*?F\C\Cn\C}D/F)?F.[B0]D 0|C|C 0"2"2;B
A 0 0assert.match(importBody,C 0/FapplySettings\C(nextSettings,C 0\C{B 0persistPreviousActiveTask:A 0false 0\C}D\C);B[B\Cs\CS]D*?Fstate\C.taskHistoryGeneration 0\C+B=D 01;B\Cs*state\C.taskContextGeneration 0\C+B=D 01;B/F);B
A}D);B
A
Atest("2ä4¸8ªAè8‚2¡1è8¡1ŒCæ6ƒ3…5é9»B˜8è8®E¤4è8¯F·7æ6±1‚2å5¤4Dç7”4¨8ç7»BŸFä4¸8€0å5¿F«Bç7…5§7ïF¼C›Bæ6—7§7æ6œCDå5ŠA¡1ä4»B…5å5œC¨8 0404 0æ6—7¶6å5›BEé9€0€0ïF¼CŒCæ6˜8¾Eå5¼CFæ6—7¥5æ6œCŸFè8ŒCƒ3å5›B´4å5°0ŠAé9‡7Dç7‹B¬Cç7«B‹Bæ6—7¥5 0K 0å5F‚2æ6•5°0"2,C 0() 0=D>E 0{B
A 0 0const 0start 0=D 0app.indexOf("2async 0function 0runQuoteQuery(event) 0{B"2);B
A 0 0const 0end 0=D 0app.indexOf("2\C\Cnfunction 0"2,C 0start 0+B 020);B
A 0 0const 0query 0=D 0start 0>E=D 00 0&6&6 0end 0>E 0start 0?F 0app.slice(start,C 0end) 0:A 0"2"2;B
A 0 0assert.ok(query,C 0"2ä4¸8»Bè8¡1ŒCæ6ƒ3…5æ6ŸF¥5è8¯F¢2æ6µ51ç7¨8‹Bå5¿F…5é9¡1»Bå5­D˜8å5œC¨8"2);B
A
A 0 0const 0defaultRange 0=D 0query.indexOf("2if 0(!startDate 0&6&6 0!endDate)"2);B
A 0 0const 0bundleRequest 0=D 0query.indexOf("2bundleResponse 0=D 0await 0getMarketBundle"2,C 0defaultRange);B
A 0 0assert.ok(defaultRange 0>E=D 00 0&6&6 0bundleRequest 0>E 0defaultRange,C 0"2é9»B˜8è8®E¤4æ6—7¥5æ6œCŸFè8ŒCƒ3å5›B´4å5ºA”4å5…5ˆ8è8¯F·7æ6±1‚2ç7»BŸFä4¸8€0è8¡1ŒCæ6ƒ3…5å5¿F«Bç7…5§7"2);B
A
A 0 0const 0fallbackStart 0=D 0query.indexOf("2if 0(error.status 0=D=D=D 0404)"2,C 0bundleRequest);B
A 0 0const 0fallbackEnd 0=D 0query.indexOf("2if 0(bundleResponse)"2,C 0fallbackStart);B
A 0 0const 0fallback 0=D 0query.slice(fallbackStart,C 0fallbackEnd);B
A 0 0assert.ok(fallbackStart 0>E 0bundleRequest 0&6&6 0fallbackEnd 0>E 0fallbackStart);B
A 0 0assert.match(fallback,C 0/FusedLegacyBundleFallback 0=D 0true/F);B
A 0 0assert.match(fallback,C 0/FPromise\C.allSettled\C(\C[B[B\Cs\CS]D*?FgetQuote\C([B\Cs\CS]D*?FgetDailyBars\C(/F);B
A 0 0assert.doesNotMatch(fallback,C 0/Felse\Cs*\C{B[B\Cs\CS]D*?FgetQuote\C(/F,C 0"2Bundle 0é9DE 0404 0å5¤4±1è8´4¥5ä4¸8Dè8ƒ3½Då5†6Dæ6¬C¡1è8§7¦6å5F‘1ä4¸8¤4ä4¸8ªA 0Provider 0è8¯F·7æ6±1‚2"2);B
A
A 0 0const 0explicitRange 0=D 0query.indexOf("2Bundle 0has 0no 0start/Fend 0query 0parameters"2);B
A 0 0assert.ok(explicitRange 0>E=D 00,C 0"2æ6˜8¾Eå5¼CFæ6—7¥5æ6œCŸFè8ŒCƒ3å5›B´4å5ºA”4è8µ5°0ç7‹B¬Cç7«B‹Bæ6ŠA¥5ä4»B·7ä4¸8Eæ6—7¥5 0K 0è8¯F·7æ6±1‚2"2);B
A 0 0const 0explicitQuery 0=D 0query.slice(explicitRange);B
A 0 0assert.match(explicitQuery,C 0/FgetQuote\C(state\C.settings,C 0symbol,C 0controller\C.signal\C)/F);B
A 0 0assert.match(explicitQuery,C 0/FgetDailyBars\C(state\C.settings,C 0symbol,C 0\C{B 0limit,C 0adjustment,C 0start:A 0startDate,C 0end:A 0endDate 0\C}D/F);B
A}D);B
A
A
Atest("2æ6•5°0æ6D®Eä4¸8­Då5¿Fƒ3è8¯F»Bå5F–6å5¹9¶6å5±1•5ç7¤4ºAç7œCŸFå5®EE 0Provider 0è8¿F0è8¡1ŒCè8¯FŠAæ6–6­DïF¼CŒCä4¸8”4ä4¸8Då5°0†6æ6œCªAè8§7‚2æ6µ5‹Bæ6 0‡7è8®E°0ä4¸8ºAå51¥5å5ºA·7"2,C 0() 0=D>E 0{B
A 0 0assert.match(apiClient,C 0/Fexport 0function 0getDataSourceDiagnostics\C(settings,C 0signal\C)/F);B
A 0 0assert.match(dataCenter,C 0/FrunHealthCheck\C(getDataSourceDiagnostics,C 0settings\C)/F);B
A 0 0assert.match(dataCenter,C 0/Ffunction 0renderProviderDiagnostics\C(payload\C)/F);B
A 0 0assert.match(dataCenter,C 0/Fnot_observed:A 0"2å5°0šAæ6—7 0ä4¸8šAå5ŠA¡1è8§7‚2æ6µ5‹B"2/F);B
A 0 0assert.match(dataCenter,C 0/FDIAGNOSTIC_STATUS\C[Bstatus\C]D/F);B
A 0 0assert.match(dataCenter,C 0/Findex_quotes:A 0"2æ6ŒC‡7æ6•5°0è8¡1ŒCæ6ƒ3…5"2/F);B
A 0 0assert.match(dataCenter,C 0/Fmarket_breadth:A 0"2å5¸8‚2å5œCºAå5¹9¿Få5ºA¦6"2/F);B
A 0 0assert.match(dataCenter,C 0/Fsector_rankings:A 0"2æ6D¿Få5D—7æ6E’2è8¡1ŒC"2/F);B
A 0 0assert.match(dataCenter,C 0/Fä4¸8Dä4¼CšAæ6µ5‹Bè8¯F•5å50ŒCè8ŠA±1é9¡1ºA 0API 0Key 0æ6ˆ8–6è8¯F·7æ6±1‚2ä4¸8ŠAæ6¸8¸8/F);B
A 0 0assert.match(dataCenter,C 0/Fç7´4¯Fè8®E¡1æ6ˆ80å5ŠAŸF/F);B
A 0 0assert.match(dataCenter,C 0/Fç7´4¯Fè8®E¡1å5¤4±1è8´4¥5/F);B
A 0 0assert.match(dataCenter,C 0/Fè8¿FEç7»B­Då5¤4±1è8´4¥5/F);B
A 0 0assert.match(dataCenter,C 0/Fè8¿F0è8¡1ŒCè8¯FŠAæ6–6­Dæ6E¥5å5F£3è8¯F»Bå5F–6å5¤4±1è8´4¥5/F);B
A 0 0assert.match(dataCenter,C 0/Fdata-center-hithink-status/F);B
A 0 0assert.match(html,C 0/FMARKET_DATA_HITHINK_ENABLED/F);B
A 0 0assert.match(html,C 0/Fid=D"2data-center-hithink-title"2/F);B
A 0 0assert.match(html,C 0/FMARKET_DATA_HITHINK_API_KEY/F);B
A 0 0assert.match(html,C 0/Fid=D"2data-center-diagnostics-list"2/F);B
A 0 0assert.match(html,C 0/Fä4¸8Dä4¸8»Bå5ŠA¨8è8°0ƒ3ç7”4¨8å50ŒCè8ŠA±1é9¡1ºAæ6E¥5å5F£3ïF¼CŒCä4¸8Dé9ªAŒCè8¯F1ç7¬C¬Cä4¸8‰9æ6–6¹9 0API 0Key/F);B
A 0 0assert.match(html,C 0/Få5F‘1é9€01æ6µ5‹Bè8¯F•5ïF¼Cˆ8å5¾E…5æ6E¥5å5…5¥5ïF¼C‰9/F);B
A 0 0assert.match(html,C 0/Fé9€0šAç7ŸF¥5æ6¸8 0é91“3ç7®E¡1ç70†6ã3€01è8®E¢2é9˜8…5é9…5Dç7½D®Eå5’2ŒCå5F‘1é9€01æ6µ5‹Bè8¯F•5æ6E¥5å5F£3/F);B
A 0 0assert.doesNotMatch(html,C 0/Få5½D“3å5‰9Dæ6²2¡1æ6œC‰9é9€00 0Provider 0\C/F 0capability 0ç7šA„4å5F—7æ6E§7ç7œCŸFå5®EEæ6E¢2æ6µ5‹B 0API/F);B
A}D);B
A
A
Atest("2è8‡7ªAé9€0‰9è8‚2¡1æ6‰9¹9é9‡7Fè8¡1ŒCæ6ƒ3…5å5FªAè8¯F·7æ6±1‚2å5½D“3å5‰9Dæ6œC¬Cå5œC°0æ6¸8…5å5D•5å5¹9¶6å5œC¨8è8¿FEæ6E¥5å5F˜8æ6›B´4æ6—7¶6æ6¸8…5é9™9¤4æ6—7§7å5¿F«Bç7…5§7"2,C 0() 0=D>E 0{B
A 0 0assert.match(apiClient,C 0/Fexport 0function 0getMarketQuotes\C(settings,C 0symbols,C 0signal\C)/F);B
A 0 0assert.match(app,C 0/Ffunction 0refreshWatchlistQuotes\C(\C)/F);B
A 0 0assert.match(app,C 0/FgetMarketQuotes\C(state\C.settings,C 0symbols,C 0controller\C.signal\C)/F);B
A 0 0assert.match(app,C 0/Fstate\C.watchlistQuotes 0=D 0nextQuotes/F);B
A 0 0assert.match(app,C 0/FinvalidateWatchlistQuotes\C(\C)/F);B
A 0 0assert.match(html,C 0/Fid=D"2refresh-watchlist-quotes"2/F);B
A 0 0assert.match(html,C 0/Fid=D"2watchlist-quote-refresh-status"2/F);B
A}D);B
A
A
Atest("2è8¿F0è8¡1ŒCæ6µ51é9‡7‡7ç7”4¨8å5…5¨8å5±1Få5FŒCæ6 0Få5·7¥5ä4½DœCå5ŒCºAïF¼CŒCè8ŠA‚2ç7‚2¹9å5’2ŒCè8´4¨8é9‡7Fè8¯FŠAæ6–6­Då5F¯Fæ6ŠA˜8å5F 0"2,C 0() 0=D>E 0{B
A 0 0assert.match(html,C 0/Fclass=D"2run-flow-workspace"2/F);B
A 0 0assert.match(html,C 0/Fclass=D"2run-flow-result-column"2/F);B
A 0 0assert.match(html,C 0/Fclass=D"2run-flow-event-inspector"2/F);B
A 0 0assert.match(app,C 0/FexpandedTaskEventSequences/F);B
A 0 0assert.match(app,C 0/Fnode\C.addEventListener\C("2toggle"2/F);B
A 0 0assert.match(app,C 0/Ftitle 0=D=D=D 0"2æ6•5°0æ6D®Eè8´4¨8é9‡7Fä4¸8Eç7­D–6ç7•5¥5é9—7¨8ç7¦61"2[B\Cs\CS]D*?Fanalysis-result-disclosure/F);B
A 0 0assert.match(stylesheet,C 0/Fgrid-template-columns:Aminmax\C(300px,C\C.82fr\C) 0minmax\C(0,C1\C.18fr\C)/F);B
A 0 0assert.match(stylesheet,C 0/F\C.run-flow-node\C[Bopen\C]D/F);B
A 0 0assert.match(stylesheet,C 0/F@0media 0\C(max-width:A900px\C)[B\Cs\CS]D*?Frun-flow-workspace/F);B
A}D);B
A
Atest("2ä4¸8ªAè8‚2¡1ç7 0”4ç7©9¶6å5ˆ8—7è8¡1¨8å5¤4Dç7”4¨8è8‡7ªAé9€0‰9è8‚2¡1æ6‰9¹9é9‡7Fè8¡1ŒCæ6ƒ3…5å5¹9¶6å5ŒCºAå5ˆ8†6å5¤4±1è8´4¥5ä4¸8Eæ6—7§7å5¿F«Bç7…5§7"2,C 0() 0=D>E 0{B
A 0 0const 0start 0=D 0app.indexOf("2function 0renderResearchStockList() 0{B"2);B
A 0 0const 0end 0=D 0app.indexOf("2\Cnfunction 0safeReadWatchlistState()"2,C 0start);B
A 0 0const 0renderer 0=D 0app.slice(start,C 0end);B
A 0 0assert.match(renderer,C 0/Fstate\C.watchlistQuotes\C.get\C(normalizeWatchlistQuoteSymbol\C(item\C.symbol\C)\C)/F);B
A 0 0assert.match(renderer,C 0/Fconst 0itemQuote 0=D 0isCurrent 0\C?F 0quote 0:A 0batchRecord\C?F\C.quote/F);B
A 0 0assert.match(renderer,C 0/FbatchQuoteFailed 0\C?F 0"2è8¡1ŒCæ6ƒ3…5å5¤4±1è8´4¥5"2 0:A 0"2æ6œCªAå5ŠA 0è8½D½D"2/F);B
A 0 0assert.match(renderer,C 0/Fæ6—7§7å5¿F«Bç7…5§7 0Â2·7 0/F);B
A 0 0assert.match(renderer,C 0/F\C[B"2loading"2,C 0"2error"2\C]D\C.includes\C(state\C.watchlistQuoteStatus\C)/F);B
A 0 0assert.match(renderer,C 0/Fæ6—7§7å5¿F«Bç7…5§7 0Â2·7 0å5ˆ8·7æ6–6°0ä4¸8­D/F);B
A 0 0assert.match(renderer,C 0/Fæ6œC€0è8¿F‘1ä4¸8€0æ6¬C¡1æ6‰9¹9é9‡7Få5ˆ8·7æ6–6°0å5¤4±1è8´4¥5ïF¼CŒCå5½D“3å5‰9Dä4¿FDç7•5™9æ6­D¤4å5‰9Dæ6ˆ80å5ŠAŸFå5¿F«Bç7…5§7/F);B
A 0 0assert.match(renderer,C 0/FformatResearchMA\C(5,C 0isCurrent\C)/F);B
A 0 0assert.match(renderer,C 0/FformatResearchMA\C(20,C 0isCurrent\C)/F);B
A}D);B
A
Atest("2å5…5¨8å5¸8‚2å5œCºAå5ˆ8†6é9¡1µ5è8¡1ŒCæ6ƒ3…5å5FªAè8°0ƒ3ç7”4¨8å5ˆ8†6é9¡1µ5æ6¦6‚2è8§7ˆ8 0API 0å5¹9¶6æ6­D£3ç7¡1®Eå5±1•5ç7¤4ºAè8¦6†6ç7›B–6ä4¸8Eè8´4¨8é9‡7Fç7ŠA¶6æ6€01"2,C 0() 0=D>E 0{B
A 0 0assert.match(app,C 0/Fimport 0\C{B 0bindMarketStocks 0\C}D 0from 0"2\C.\C/Fmarket-stocks\C.js"2/F);B
A 0 0assert.match(app,C 0/FbindMarketStocks\C([B\Cs\CS]D*?FselectStockFromMarket\C(item,C 0"2å5…5¨8å5¸8‚2å5œCºAè8¡1ŒCæ6ƒ3…5"2\C)/F);B
A 0 0assert.match(marketStocksModule,C 0/FgetMarketStocks\C(settings,C 0\C{B/F);B
A 0 0assert.match(marketStocksModule,C 0/Fcoverage_ratio/F);B
A 0 0assert.match(marketStocksModule,C 0/Ftotal_is_lower_bound/F);B
A 0 0assert.match(marketStocksModule,C 0/Fitem\C?F\C.ma5/F);B
A 0 0assert.match(marketStocksModule,C 0/Fitem\C?F\C.ma20/F);B
A 0 0assert.match(marketStocksModule,C 0/Fquote\C?F\C.quality_status/F);B
A 0 0assert.match(marketStocksModule,C 0/Fcontroller\C.signal\C.aborted 0\C|C\C|C 0generation 0!=D=D 0state\C.generation/F);B
A 0 0assert.match(marketStocksModule,C 0/Fstate\C.page 0\C* 0state\C.pageSize 0<C 0total/F);B
A 0 0assert.match(app,C 0/Få5¤4±1æ6•5ˆ8å5…5¨8å5¸8‚2å5œCºAå5ˆ8†6é9¡1µ5è8¡1ŒCæ6ƒ3…5å5¤4±1è8´4¥5ïF¼CšA[B\Cs\CS]D*?FmarketStocks\C.invalidate\C(\C)/F);B
A 0 0assert.match(marketStocksModule,C 0/FqualityPill\C.title 0=D 0qualityReasons\C.join/F);B
A 0 0assert.match(marketStocksModule,C 0/Fprev\C.disabled 0=D 0busy 0\C|C\C|C 0state\C.page 0<C=D 01/F);B
A 0 0assert.match(html,C 0/Fid=D"2market-stocks-page-size"2/F);B
A 0 0assert.match(html,C 0/Fid=D"2market-stocks-adjustment"2/F);B
A 0 0assert.match(html,C 0/Fid=D"2market-stocks-prev"2/F);B
A 0 0assert.match(html,C 0/Fid=D"2market-stocks-next"2/F);B
A 0 0assert.match(html,C 0/Få5½D“3å5‰9Dé9¡1µ5è8¡1ŒCæ6ƒ3…5è8¦6†6ç7›B–6/F);B
A 0 0assert.match(apiClient,C 0/Fexport 0function 0getMarketStocks/F);B
A 0 0assert.match(marketOpenAPI,C 0/Fcoverage_ratio:A 0\C{Btype:A 0number/F);B
A 0 0assert.match(marketOpenAPI,C 0/Ftotal_is_lower_bound:A 0\C{Btype:A 0boolean/F);B
A}D);B
A
Atest("2å5¸8‚2å5œCºAè8‚2¡1ç7¥5¨8ä4¸8Eæ6ƒ3…5ç7»BªAå5E†6å5F²2 0API 0å5°01è8£3…5ä4¿FDæ6ŒC1ç7œCŸFå5®EEæ6E¥5å5F£3å5¥5‘1ç7ºA¦6"2,C 0() 0=D>E 0{B
A 0 0assert.match(apiClient,C 0/Fexport 0function 0getMarketStocks/F);B
A 0 0assert.match(apiClient,C 0/F\C/Fapi\C/Fv1\C/Fmarket\C/Fstocks\C?F/F);B
A 0 0assert.match(apiClient,C 0/Fpage_size:A 0String\C(normalizedPageSize\C)/F);B
A 0 0assert.match(apiClient,C 0/Fexport 0function 0getMarketSentiment\C(/F);B
A 0 0assert.match(apiClient,C 0/F\C/Fapi\C/Fv1\C/Fmarket\C/Fsentiment/F);B
A 0 0assert.match(apiClient,C 0/Fexport 0function 0getMarketSentimentHistory/F);B
A 0 0assert.match(apiClient,C 0/F\C/Fapi\C/Fv1\C/Fmarket\C/Fsentiment\C/Fhistory\C?F/F);B
A 0 0assert.match(marketHTTPServer,C 0/Fmux\C.Handle\C("2GET 0\C/Fapi\C/Fv1\C/Fmarket\C/Fstocks"2/F);B
A 0 0assert.match(marketHTTPServer,C 0/Fmux\C.Handle\C("2GET 0\C/Fapi\C/Fv1\C/Fmarket\C/Fsentiment"2/F);B
A 0 0assert.match(marketHTTPServer,C 0/Fmux\C.Handle\C("2GET 0\C/Fapi\C/Fv1\C/Fmarket\C/Fsentiment\C/Fhistory"2/F);B
A 0 0assert.match(marketOpenAPI,C 0/F\C/Fapi\C/Fv1\C/Fmarket\C/Fstocks:A/F);B
A 0 0assert.match(marketOpenAPI,C 0/F\C/Fapi\C/Fv1\C/Fmarket\C/Fsentiment:A/F);B
A 0 0assert.match(marketOpenAPI,C 0/F\C/Fapi\C/Fv1\C/Fmarket\C/Fsentiment\C/Fhistory:A/F);B
A 0 0assert.match(html,C 0/Fid=D"2market-sentiment-history-start"2/F);B
A 0 0assert.match(html,C 0/Fid=D"2market-sentiment-history-end"2/F);B
A 0 0assert.match(app,C 0/FgetMarketSentimentHistory/F);B
A 0 0assert.match(app,C 0/Fä4¸8Dä4¼CšAç7”4¨8å5½D“3å5‰9Dè8¡1ŒCæ6ƒ3…5è8¡1¥5é9€0 0å5E†6å5F²2/F);B
A}D);B
A
Atest("2è8®E¾Eç7½D®Eä4¸8­Då5¿Fƒ3å5¯F¼Cè8ˆ8ªAã3€01å5ŒCºAå5D—7å5’2ŒCæ6œC¬Cå5œC°0é9…5Dç7½D®Eä4ºA¤4ä4ºA’2ä4¿FDæ6ŒC1ä4¸8€0è8‡7´4"2,C 0() 0=D>E 0{B
A 0 0const 0targets 0=D 0[B...html.matchAll(/Fdata-settings-target=D"2([B^E"2]D+B)"2/Fg)]D.map((match) 0=D>E 0match[B1]D);B
A 0 0const 0sections 0=D 0[B...html.matchAll(/F<Csection[B^E>E]D*class=D"2[B^E"2]D*settings-section[B^E"2]D*"2[B^E>E]D*id=D"2([B^E"2]D+B)"2/Fg)]D.map((match) 0=D>E 0match[B1]D);B
A 0 0assert.equal(targets.length,C 010,C 0"2è8®E¾Eç7½D®Eä4¸8­Då5¿Fƒ3å5ºA”4ä4¿FDç7•5™9 010 0ä4¸8ªAå5·7²2è8§7„4å5ˆ8’2å5ˆ8†6ç7»B„4"2);B
A 0 0assert.deepEqual([B...targets]D.sort(),C 0[B...sections]D.sort(),C 0"2æ6¯FFä4¸8ªAè8®E¾Eç7½D®Eå5¯F¼Cè8ˆ8ªAå5¿F…5é9¡1»Bå5”4¯Fä4¸8€0å5¯F¹9å5ºA”4ä4¸8€0ä4¸8ªAå5†6…5å5®E¹9å5ŒCºAå5D—7"2);B
A 0 0assert.equal(new 0Set(targets).size,C 0targets.length,C 0"2è8®E¾Eç7½D®Eå5¯F¼Cè8ˆ8ªAç7›B®Eæ6 0‡7ä4¸8Dè8ƒ3½Dé9‡7Då5¤4D"2);B
A 0 0assert.doesNotMatch(html,C 0/F<C\C/F(?F:Asection|Cdiv|Cmain|Cform|Cnav|Carticle|Caside|Cfooter)>E>E/F,C 0"2HTML 0ä4¸8Dè8ƒ3½Då5ŒC…5å50«Bå5¤4šAä4½D™9ç7šA„4é9—7­Då50ˆ8æ6 0‡7ç7­D¾Eå5­D—7ç7¬C¦6"2);B
A 0 0assert.match(app,C 0/FresetPreferences\C(\C)/F,C 0"2æ61¢2å5¤4Dæ6œC¬Cå5œC°0å51Få5¥5½Då5¿F…5é9¡1»Bè8°0ƒ3ç7”4¨8ç7»BŸFä4¸8€0ç7šA„4æ6œC¬Cå5œC°0å51Få5¥5½Då5­D˜8å5‚2¨8æ6¨8¡1å5D—7"2);B
A 0 0assert.match(app,C 0/FbindNotificationPreferences\C(/F,C 0"2é9€0šAç7ŸF¥5æ6œC¬Cå5œC°0å51Få5¥5½Då5¿F…5é9¡1»Bé9€0šAè8¿F‡7ç7‹B¬Cç7«B‹Bæ6¨8¡1å5D—7ç7»B‘1å5®EšA"2);B
A 0 0assert.match(html,C 0/Fä4¿FDå5­D˜8æ6œC¬Cå5œC°0å51Få5¥5½D/F);B
A 0 0assert.match(html,C 0/Fä4¿FDå5­D˜8æ6¸8 0é91“3ïF¼Cˆ8å5¾E…5æ6E¥5å5…5¥5ïF¼C‰9/F);B
A 0 0assert.match(html,C 0/Få5F‘1é9€01æ6µ5‹Bè8¯F•5ïF¼Cˆ8å5¾E…5æ6E¥5å5…5¥5ïF¼C‰9/F);B
A}D);B
A
Atest("2è8¡1ŒCæ6ƒ3…5æ6œCDå5ŠA¡1æ6E¥5å5F£3æ6–6‡7æ6¡1£3ã3€01Handler 0ä4¸8E 0WebUI 0è8°0ƒ3ç7”4¨8è8¾E¹9ç7•5ŒCæ6˜8Eç7¡1®Eå5ŒCºAå5ˆ8†6"2,C 0() 0=D>E 0{B
A 0 0assert.match(marketHTTPServer,C 0/Fmux\C.HandleFunc\C("2GET 0\C/Fhealth\C/Flive"2/F);B
A 0 0assert.match(marketHTTPServer,C 0/Fmux\C.HandleFunc\C("2GET 0\C/Fhealth\C/Fready"2/F);B
A 0 0assert.match(marketHTTPServer,C 0/Fmux\C.Handle\C("2GET 0\C/Fapi\C/Fv1\C/Fdata-sources\C/Fdiagnostics"2/F);B
A 0 0assert.match(marketHTTPServer,C 0/Fmux\C.Handle\C("2POST 0\C/Fapi\C/Fv1\C/Fdata-sources\C/Fprobe"2/F);B
A 0 0assert.match(apiClient,C 0/FgetDataSourceDiagnostics/F);B
A 0 0assert.doesNotMatch(apiClient,C 0/Fdata-sources\C/Fprobe/F,C 0"2WebUI 0ä4¸8Då5ºA”4ä4¸8ºAå50ŒCè8ŠA±1é9¡1ºAè8¯FŠAæ6–6­Dè8°0ƒ3ç7”4¨8ä4¸8»Bå5ŠA¨8æ6E¢2æ6µ5‹Bæ6E¥5å5F£3"2);B
A 0 0assert.match(marketOpenAPI,C 0/F\C/Fapi\C/Fv1\C/Fmarket\C/Fcapital-flow:A/F,C 0"2OpenAPI 0å5½D“3å5‰9Då5£3°0æ6˜8Eä4ºA†6å5¸8‚2å5œCºAç7ºA§7è8µ5„4é9‡7‘1æ6µ51æ6E¥5å5F£3"2);B
A 0 0assert.doesNotMatch(marketHTTPServer,C 0/Fmux\C.Handle\C("2GET 0\C/Fapi\C/Fv1\C/Fmarket\C/Fcapital-flow"2/F,C 0"2å5¸8‚2å5œCºAç7ºA§7è8µ5„4é9‡7‘1æ6µ51å5°0šAæ6—7 0 0Handler 0æ6³3¨8å5†6ŒC"2);B
A 0 0assert.match(marketHTTPServer,C 0/Fmux\C.Handle\C("2GET 0\C/Fapi\C/Fv1\C/Fstocks\C/F\C{Bsymbol\C}D\C/Fcapital-flow"2/F,C 0"2ä4¸8ªAè8‚2¡1è8µ5„4é9‡7‘1æ6µ51 0Handler 0å5·7²2æ6³3¨8å5†6ŒC"2);B
A 0 0assert.match(apiClient,C 0/Fexport 0function 0getCapitalFlow/F);B
A 0 0assert.doesNotMatch(apiClient,C 0/Fexport 0function 0getMarketCapitalFlow/F,C 0"2WebUI 0ä4¸8Då5ºA”4æ6ŠAŠAæ6œCªAæ6³3¨8å5†6ŒCç7šA„4å5¸8‚2å5œCºAç7ºA§7è8µ5„4é9‡7‘1æ6µ51æ6E¥5å5F£3å5½D“3ä4½DœCå5F¯Fç7”4¨8è8ƒ3½Då5ŠA›B"2);B
A}D);B
A
A
Atest("2è8¿F0è8¡1ŒCæ6µ51è8¯F¦6æ6ƒ3…5ä4¸8Då5†6Dé9˜8»Bæ6–6­Då5·7¦6ä4¾E§7ä4¸8»Bå5¯F¼Cè8ˆ8ªAç7‚2¹9å5‡7»B"2,C 0() 0=D>E 0{B
A 0 0assert.match(app,C 0/Ffunction 0openRunFlowDrawer\C(\C)[B\Cs\CS]D*?Fdialog\C.show\C(\C)/F);B
A 0 0assert.match(stylesheet,C 0/F\C.run-flow-dialog\C[Bopen\C]D\Cs*\C{B[B^E}D]D*pointer-events:A\Cs*none/F);B
A 0 0assert.match(stylesheet,C 0/F\C.run-flow-dialog\C[Bopen\C]D\Cs*\C.task-status-panel\Cs*\C{B\Cs*pointer-events:A\Cs*auto/F);B
A}D);B
A