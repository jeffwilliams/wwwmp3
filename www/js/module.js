/* mp3 class */
function Mp3() {
  this.artist = ""
  this.album = ""
  this.track = ""
}
/* End mp3 class */

function hashKeys(hash){
  var l = [];
  for(var key in hash){
    l.push(key);
  }
  return l;
}

function beginsWith(string, substring)
{
  var l = substring.length;
  return string.length >= l && string.substring(0, substring.length) == substring;
}

function matchCriteria(crit, s) {
  return crit == "" || s.toLowerCase().indexOf(crit) > -1;
}

// Convert a number of seconds to a time format string
function secondsToTime(secs) {
  var s = Math.round(secs % 60.0);
  var m = Math.floor(secs / 60.0);
  var h = null;

  if(m > 60) {
    m = m % 60.0;
    h = Math.floor(m / 60.0);
  }
  
  // Convert to two-digit strings
  var result = "";
  if(s < 10) {
    s = "0" + s;
  }
  result += s;

  if (m < 10) {
    m = "0" + m;
  }
  result = m + ":" + result

  if (h){
    if (h < 10) {
      h = "0" + h;
    }
    result = h + ":" + result
  }

  return result;
}

/*
* This class attempts to prevent an operation from being performed too often.
* Callers should call send() to perform the operation, but send will only occur
* as often as the delay period.
*/
function Throttler($timeout, delay, set){

  this.$timeout = $timeout;

  // Function to call to set the value
  this.set = set;

  this.delay = delay;

  this.timer = null;

  this.send = function(){
    if(this.timer){
      // Cancel previous job
      this.$timeout.cancel(this.timer);
    }
    
    this.timer = this.$timeout(this.set, this.delay)
    this.timer.finally(
      function(){
        this.timer = null;
      }
    );
  }
}

/*
 * This class keeps track of which page of results we were on for each previous
 * shorter search string. 
 */
function SearchStringPage(){
  // Array indexed by search string size that contains the page number we were on when 
  // the search string was that size. 
  //
  // For example if the search string was empty, the user then changed from page 0 to
  // page 3, 'a' was entered, the user then changed from page 0 to
  // page 1, then entered 'm', this array would contain [3,1].
  //  
  this.pages = [];

  this.reset = function() {
    this.pages = [];
  }

  this.update = function(searchlen, page) {
    // If the search string length is smaller than the entries we have in pages, then
    // we can shrink pages because the user has hit backspace to truncate the search string.
    while (this.pages.length > searchlen+1){
      this.pages.pop();
    }
    // If the search string length is greater than the entries we have in pages, then
    // we need to add new entries.
    while (this.pages.length <= searchlen){
      this.pages.push(0);
    }

    this.pages[searchlen] = page;
  }

  // Get the page for the searchlen
  this.get = function(searchlen) {
    if (searchlen >= this.pages.length) {
      return 0;
    }
    return this.pages[searchlen];
  }
}

/*
Parameters: Pass a variable number of [property, criteria] pairs.
This function returns a new function f. f takes one argument, a Mp3 structure,
and returns true if Mp3 matches each of the [property, criteria] pairs. For each
critera,property pair, mp3.property is compared against criteria, and if the criteria
doesn't match false is returned.

*/
function matcher() {

  var args = arguments;

  return function(song) {
    for(var i = 0; i < args.length; i++){
      var arg = args[i];
      var prop = arg[0];
      var crit = arg[1];

      if( crit != "" && song[prop].toLowerCase().indexOf(crit.toLowerCase()) < 0 ){
        return false;
      }
    }

    return true;
  }
}

/*
Return a list of songs that 
match the 'matches' predicate. matches should be a function that takes a song as input
and returns true or false.
*/
function filterSongs(songs, matches) {
  var rc = [];
  for(var i = 0; i < songs.length; i++){
    var s = songs[i];
    if (matches(s)) {
      rc.push(s);
    }
  }
  return rc;
}

/*
Return a list of all unique values of the property prop from the objects in list objs.
*/
function props(objs, prop) {
  var rc = {};
  for(var i = 0; i < objs.length; i++){
     rc[objs[i][prop]] = 1;
  }
  return hashKeys(rc);
}

function propFromFilteredSongs(songs, matches, prop) {
  var v = props(
    filterSongs(
      songs,
      matches),
    prop);
  return v;
}

function findSong(songs, artist, album, title) {
  return filterSongs(songs, matcher(["artist",artist],["album",album],["title",title]))
}

/*
Get a page of data from a list. Returns a two element list:
  1. The page of data
  2. The index of the page returned.
*/
function page(list, pagesize, page){
  var rc = [];

  var total = totalPages(list, pagesize);

  if(page >= total){
    page = total-1;
  }
  if(page < 0){
    page = 0;
  }

  var start = page*pagesize;

  for(var i = 0; i < pagesize && i + start < list.length; i++){
    rc.push(list[i+start]);
  }

  return [rc, page];
}

function totalPages(list, pagesize){
  var pages = list.length/pagesize;
  var completePages = Math.floor(pages);
  var totalPages = completePages;
  if (pages > completePages) {
    totalPages = completePages+1;
  }
  return totalPages;
}

/*** Angular JS ***/

var playerModule = angular.module('player',[]);

playerModule.controller('MainCtrl', MainCtrl)

function MainCtrl($scope, $http, $timeout){
  //$scope.songs = sample_data();

  $scope.artistCriteria = "";
  $scope.albumCriteria = "";
  $scope.titleCriteria = "";

  $scope.artistPage = 0;
  $scope.artistPageIsLast = true;
  $scope.albumPage = 0;
  $scope.albumPageIsLast = true;
  $scope.titlePage = 0;
  $scope.titlePageIsLast = true;

  $scope.artistFormerPages = new SearchStringPage();
  $scope.albumFormerPages = new SearchStringPage();
  $scope.titleFormerPages = new SearchStringPage();

  $scope.volume = 50;

  // Position in the file
  $scope.position = 50;
  $scope.maxPosition = 100;
  $scope.seekingToPosition = null;
  $scope.seekedAt = null;

  $scope.artists = []
  $scope.albums = []
  $scope.songs = []

  // Currently playing mp3
  $scope.playing = null;
  // State of the currently playing mp3
  $scope.state = null;

  $scope.playQueue = [];
  // Make a unique key from a playqueue item
  //$scope.metadataMakeKey = function(v){ return v.path; }
  $scope.selectionListKeyFromQueueId = function(v){ return v.queueId; }
  $scope.selectionListKeyFromPath = function(v){ return v.path; }

  $scope.recentlyPlayed = [];

  $scope.playerEventsWebsock = null;

  $scope.scannedMp3 = null;

  // Repeat mode is one of: DontRepeat, RepeatOne, or RepeatAll
  $scope.repeatMode = "DontRepeat";


  // Get a printable version of the last scanned mp3
  $scope.scannedMp3ForDisplay = function() {
    if(null == $scope.scannedMp3){
      return "";
    } else {
      return $scope.scannedMp3.Title + " by " + $scope.scannedMp3.Artist;
    }
  }

  /**************** PLAYER EVENT HANDLING ******************/
  var handlePlayerOffsetEvent = function(position){
    var now = (new Date()).getTime();
    if($scope.seekingToPosition != null && ($scope.seekingToPosition == position || $scope.seekedAt <= (now - 1000))){
      $scope.seekedAt = null;
      $scope.seekingToPosition = null;
    }

    if($scope.position != position && $scope.seekingToPosition == null){
      $timeout(function(){
        $scope.position = position;
      });
    }
  }

  // Called when the server tells us the volume has changed
  var handlePlayerVolumeEvent = function(volume){
    if($scope.volume != volume){
      // Since this is triggered outside Angular, we need to use $timeout to trigger
      // angular to detect that the view was updated.
      $timeout(function(){
        $scope.volume = volume;
      });
    }
  }

  var handlePlayerSizeEvent = function(maxPosition){
    if($scope.maxPosition != maxPosition){
      // Since this is triggered outside Angular, we need to use $timeout to trigger
      // angular to detect that the view was updated.
      $timeout(function(){
        $scope.maxPosition = maxPosition;
      });
    }
  }

  var handlePlayerMetaEvent = function(meta){
    $timeout(function(){
      $scope.playing = meta;
      if (meta && ("rate" in meta)) {
        // Convert to khz
        $scope.playing.rate = $scope.playing.rate/1000 + "khz"
      }
      if (meta && ("duration" in meta)) {
        $scope.playing.duration = secondsToTime($scope.playing.duration)
      }
    });
  }

  var handlePlayerScanEvent = function(meta){
    console.log("Scan: got meta: ");
    console.log(meta);
    $timeout(function(){
      $scope.scannedMp3 = meta;
    });
  }

  var handlePlayerStateEvent = function(state){
    /* State returned from the server is an enumeration with the values:
        0 = empty
        1 = playing
        2 = paused
     */
    $timeout(function(){
      if(state == 0){
        $scope.state = null;
        $scope.playing = null;
        //$scope.playNext();
      } else if (state == 1 ){
        $scope.state = "playing";
      } else {
        $scope.state = "paused";
      }
    });
  }

  var handlePlayerQueueChangeEvent = function(meta){
    $timeout(function(){
      var oldPlayQueue = $scope.playQueue;
      $scope.playQueue = meta;
      $scope.selectionListReplaced(oldPlayQueue, $scope.playQueue, $scope.selectionListKeyFromQueueId);
    });
  }

  var handlePlayerRecentChangeEvent = function(meta){
    $timeout(function(){
      var old = $scope.recentlyPlayed;
      $scope.recentlyPlayed = meta;
      $scope.selectionListReplaced(old, $scope.recentlyPlayed, $scope.selectionListKeyFromPath);
    });
  }

  var handlePlayerRepeatModeEvent = function(repeat){
    $timeout(function(){
      $scope.repeatMode = repeat;
    });
  }

  var playerEventsConnect = function(){
    // Build the websocket URL based on the current window location.
    var loc = window.location, new_uri;
    new_uri = "ws://" + loc.host;
    new_uri += "/playerEvents";

    //socket = new WebSocket("ws://andor:2001/events");
    $scope.playerEventsWebsock = new WebSocket(new_uri);

    $scope.playerEventsWebsock.onmessage = function(event){

      // Update the position in the file
      console.log("Got player event:")
      console.log(event.data);

      var e = angular.fromJson(event.data);
      if("Volume" in e) 
        handlePlayerVolumeEvent(e["Volume"])
      if("Size" in e) 
        handlePlayerSizeEvent(e["Size"])
      if("Offset" in e) 
        handlePlayerOffsetEvent(e["Offset"])
      if("Meta" in e) 
        handlePlayerMetaEvent(e["Meta"])
      if("Scan" in e) 
        handlePlayerScanEvent(e["Scan"])
      if("State" in e) 
        handlePlayerStateEvent(e["State"])
      if("Queue" in e)
        handlePlayerQueueChangeEvent(e["Queue"])
      if("Recent" in e)
        handlePlayerRecentChangeEvent(e["Recent"])
      if("RepeatMode" in e)
        handlePlayerRepeatModeEvent(e["RepeatMode"])
    }

    $scope.playerEventsWebsock.onopen = function(event){
      console.log("Player offsets: Connected to server")
    }

    $scope.playerEventsWebsock.onclose = function(event){
      console.log("Player offsets: Connection to server lost")
      window.setTimeout(function(){
        playerEventsConnect();
      }
      , 1000)
    }

    $scope.playerEventsWebsock.onerror = function(event){
      console.log("Player offsets: socket error: " + event);
    }
  }
  /**************** END PLAYER EVENT HANDLING ******************/

  /**************** MP3 METAINFORMATION ******************/

  var getMp3Data = function(page, fields, orderFields, callback) {
    var parms = {
      'pagesize': 10,
      'page': page,
      'artist' : $scope.artistCriteria,
      'album' : $scope.albumCriteria,
      'title' : $scope.titleCriteria,
      'order' : orderFields.join()
    }

    if ( fields ){
      parms['fields'] = fields.join();
    }

    $http.get("/songmeta", {'params' : parms}).
      success(function(data,status,headers,config){
        callback(data)
        //$scope.name = data;
      }).
      error(function(data,status,headers,config){
        console.log("Error: getting song metadata failed: " + data);
      });
  }

  var getArtists = function(){
    getMp3Data($scope.artistPage, ["artist"], ["artist"], function(data){
      $scope.artistPageIsLast = false;
      $scope.artists = []
      for(var i = 0; i < data.length; i++){
        if ( data[i].eof ) {
          $scope.artistPageIsLast = true;
        } else {
          $scope.artists.push( data[i].artist )
        }
      }
    })
  }

  var getAlbums = function(){
    getMp3Data($scope.albumPage, ["album"], ["album"], function(data){
      $scope.albumPageIsLast = false;
      $scope.albums = []
      for(var i = 0; i < data.length; i++){
        if ( data[i].eof ) {
          $scope.albumPageIsLast = true;
        } else {
          $scope.albums.push( data[i].album )
        }
      }
    })
  }

  var getSongs = function(){
    getMp3Data($scope.titlePage, null, ["tracknum", "title"], function(data){
      $scope.titlePageIsLast = false;
      $scope.songs = []
      for(var i = 0; i < data.length; i++){
        if ( data[i].eof ) {
          $scope.titlePageIsLast = true;
        } else {
          $scope.songs.push( data[i] )
        }
      }
    })
  }
  
  // Update the artist, album, and title lists based on the current filter. If setPage is not null,
  // it is called after pages are initialized to 0 to set the pages to the correct value.
  var reloadMp3Metainfo = function(setPage) {
    $scope.artistPage = 0;
    $scope.albumPage = 0;
    $scope.titlePage = 0;

    if (setPage) {
      setPage();
    }

    getArtists();
    getAlbums();
    getSongs();
  }


  $scope.setArtistCriteria = function(s) {
    if ($scope.artistCriteria == '' && s == '' && $scope.artistPage != 0) {
      $scope.artistFormerPages.reset();
      reloadMp3Metainfo(null);
    }

    $scope.artistCriteria = s;
  }

  $scope.setAlbumCriteria = function(s) {
    $scope.albumCriteria = s;
  }

  $scope.setTitleCriteria = function(s) {
    $scope.titleCriteria = s;
  }

  $scope.changeArtistPage = function(delta){
    var oldpage = $scope.artistPage;
    $scope.artistPage = $scope.artistPage + delta;
    if ($scope.artistPage < 0){
      $scope.artistPage = 0;
    }

    if ( $scope.artistPageIsLast && $scope.artistPage > oldpage ){
      $scope.artistPage = oldpage;
    }

    if ($scope.artistPage != oldpage){
      getArtists();
      $scope.artistFormerPages.update($scope.artistCriteria.length, $scope.artistPage);
    }
  }

  $scope.changeAlbumPage = function(delta){
    var oldpage = $scope.albumPage;
    $scope.albumPage = $scope.albumPage + delta;
    if ($scope.albumPage < 0){
      $scope.albumPage = 0;
    }

    if ( $scope.albumPageIsLast && $scope.albumPage > oldpage ){
      $scope.albumPage = oldpage;
    }

    if ($scope.albumPage != oldpage){
      getAlbums();
      $scope.albumFormerPages.update($scope.albumCriteria.length, $scope.albumPage);
    }
  }

  $scope.changeTitlePage = function(delta){
    var oldpage = $scope.titlePage;
    $scope.titlePage = $scope.titlePage + delta;
    if ($scope.titlePage < 0){
      $scope.titlePage = 0;
    }

    if ( $scope.titlePageIsLast && $scope.titlePage > oldpage ){
      $scope.titlePage = oldpage;
    }

    if ($scope.titlePage != oldpage){
      getSongs();
      $scope.titleFormerPages.update($scope.titleCriteria.length, $scope.titlePage);
    }
  }
  /**************** END MP3 METAINFORMATION ******************/

  /**************** PLAYER REQUESTS ******************/
  var playerLoadMp3 = function(path, callback){
    var parms = {
      'load': path
    }

    $http.get("/player", {'params' : parms}).
      success(function(data,status,headers,config){
        if(data.size){
          $scope.maxPosition = data.size;
        }

        if(callback){
          callback();
        }
      }).
      error(function(data,status,headers,config){
        console.log("Error: loading mp3 failed: " + data);
      });
  }

  var playerPlayMp3 = function(){
    $http.get("/player/play").
      success(function(data,status,headers,config){
      }).
      error(function(data,status,headers,config){
        console.log("Error: playing mp3 failed: " + data);
      });
  }

  var playerQueueMp3 = function(path){
    var parms = {
      'file': path
    }

    $http.post("/player/queue.enqueue", parms).
      success(function(data,status,headers,config){
      }).
      error(function(data,status,headers,config){
        console.log("Error: enqueueing mp3 failed: " + data);
      });
  }

  var playerPauseMp3 = function(){
    $http.get("/player/pause").
      success(function(data,status,headers,config){
      }).
      error(function(data,status,headers,config){
        console.log("Error: pausing mp3 failed: " + data);
      });
  }

  var playerStopMp3 = function(){
    var parms = {
      'stop': 'stop'
    }

    $http.get("/player/stop").
      success(function(data,status,headers,config){
      }).
      error(function(data,status,headers,config){
        console.log("Error: stopping mp3 failed: " + data);
      });
  }

  var playerGetVolume = function(){
    $http.get("/player/volume").
      success(function(data,status,headers,config){
        $scope.volume = data.volume
      }).
      error(function(data,status,headers,config){
        console.log("Error: getting volume failed: " + data);
      });
  }

  var playerMoveInQueue = function(indexes, delta){
    var parms = {
      'indexes': indexes,
      'delta': delta
    }

    $http.post("/player/queue.move", parms).
      success(function(data,status,headers,config){
      }).
      error(function(data,status,headers,config){
        console.log("Error: changing queue failed: " + data);
      });
  }

  var playerMoveToTopInQueue = function(indexes){
    var parms = {
      'indexes': indexes
    }

    $http.post("/player/queue.move_to_top", parms).
      success(function(data,status,headers,config){
      }).
      error(function(data,status,headers,config){
        console.log("Error: changing queue failed: " + data);
      });
  }

  var playerRemoveFromQueue = function(indexes){
    var parms = {
      'indexes': indexes
    }

    $http.post("/player/queue.remove", parms).
      success(function(data,status,headers,config){
      }).
      error(function(data,status,headers,config){
        console.log("Error: changing queue failed: " + data);
      });
  }

  var playerClearQueue = function(){
    var parms = {
      'queue.clear': 'y'
    }

    $http.get("/player", {'params' : parms}).
      success(function(data,status,headers,config){
      }).
      error(function(data,status,headers,config){
        console.log("Error: changing queue failed: " + data);
      });
  }

  var volumeThrottler = new Throttler($timeout, 80, 
    function(){
      var parms = {
        "volume": parseInt($scope.volume)
      }

      $http.post("/player/volume", parms).
        success(function(data,status,headers,config){
        }).
        error(function(data,status,headers,config){
          console.log("Error: setting volume failed: " + data);
        });
 
    }
  );

  var playerSetVolume = function(){
    volumeThrottler.send();
  }

  var sendPlayerSeekRequest = function(){
    var parms = {
      'seek': parseInt($scope.position)
    }

    // Mark that we are seeking to a specific position so that 
    // we ignore any intermediate offset change events until we have 
    // received an event with that position. This prevents the button from 
    // jumping around.
    $scope.seekingToPosition = $scope.position;
    $scope.seekedAt = (new Date()).getTime();

    $http.post("/player/seek", parms).
      success(function(data,status,headers,config){
      }).
      error(function(data,status,headers,config){
        console.log("Error: seeking mp3 failed: " + data);
        $scope.seekingToPosition = null;
      });
  }

  var seekThrottler = new Throttler($timeout, 80, sendPlayerSeekRequest);

  var playerSeek = function(){
    seekThrottler.send();
  }

  $scope.scan = function(){
    $http.get("/scan/start").
      success(function(data,status,headers,config){
      }).
      error(function(data,status,headers,config){
        console.log("Error: scanning request failed: " + data);
      });
  }

  $scope.sendPlayerSetRepeatModeRequest = function(mode){
    var parms = {
      'mode': mode
    }

    $http.post("/player/repeat_mode", parms).
      success(function(data,status,headers,config){
      }).
      error(function(data,status,headers,config){
        console.log("Error: setting repeat mode failed: " + data);
      });
  }

  /**************** END PLAYER REQUESTS ******************/
  
  // Initial data load
  getArtists();
  getAlbums();
  getSongs();
  playerGetVolume();

  // Connect websocket
  playerEventsConnect();

  // Whenever one of our filters changes, reload the list of songs to match the filters.
  var filtersChanged = function(newValue, oldValue, setPage){
    if(newValue != oldValue){
      var callback = null;

      if(newValue.length < oldValue.length){
        callback = setPage;
      }
  
      reloadMp3Metainfo(setPage);
    }
  }

  var artistFilterChanged = function(newValue, oldValue){
    filtersChanged(newValue, oldValue, function(){
      $scope.artistPage = $scope.artistFormerPages.get(newValue.length);
    });
  }

  var albumFilterChanged = function(newValue, oldValue){
    filtersChanged(newValue, oldValue, function(){
      $scope.albumPage = $scope.albumFormerPages.get(newValue.length);
    });
  }

  var titleFilterChanged = function(newValue, oldValue){
    filtersChanged(newValue, oldValue, function(){
      $scope.titlePage = $scope.titleFormerPages.get(newValue.length);
    });
  }


  $scope.$watch('artistCriteria', artistFilterChanged);
  $scope.$watch('albumCriteria', albumFilterChanged);
  $scope.$watch('titleCriteria', titleFilterChanged);

  $scope.volumeMouseup = function(){
    playerSetVolume();
  }

  $scope.positionMouseup = function(){
    playerSeek();
  }

  /**************** PLAY QUEUE ******************/
  $scope.addSelectedToPlayQueue = function() {
    for(var i = 0; i < $scope.songs.length; i++) {
      if($scope.selectionListIsSelected($scope.songs, i)) {
        //$scope.selectionListSelect($scope.recentlyPlayed, i);
        playerQueueMp3($scope.songs[i].path);
        $scope.selectionListUnselect($scope.songs, i);
      }
    }
  }

  $scope.clearPlayQueue = function() {
    playerClearQueue();
  }

  $scope.playQueueMoveClicked = function(delta) {
    var indexes = $scope.selectionListSelectedIndexes($scope.playQueue);
    playerMoveInQueue(indexes, delta);
  }

  $scope.playQueueMoveTop = function() {
    var indexes = $scope.selectionListSelectedIndexes($scope.playQueue);
    playerMoveToTopInQueue(indexes);
  }

  $scope.playQueueSelectNone = function() {
    for(var i = 0; i < $scope.playQueue.length; i++) {
      if($scope.selectionListIsSelected($scope.playQueue, i)) {
        $scope.selectionListUnselect($scope.playQueue, i) 
      }
    }
  }

  $scope.playQueueSelectAll = function() {
    for(var i = 0; i < $scope.playQueue.length; i++) {
      if(!$scope.selectionListIsSelected($scope.playQueue, i)) {
        $scope.selectionListSelect($scope.playQueue, i);
      }
    }
  }

  $scope.playQueueRemoveClicked = function() {
    var indexes = $scope.selectionListSelectedIndexes($scope.playQueue);
    playerRemoveFromQueue(indexes);
  }
  /**************** END PLAY QUEUE ******************/

  /**************** RECENTLY PLAYED LIST ******************/
  $scope.addRecentToPlayQueue = function() {
    for(var i = 0; i < $scope.recentlyPlayed.length; i++) {
      if($scope.selectionListIsSelected($scope.recentlyPlayed, i)) {
        playerQueueMp3($scope.recentlyPlayed[i].path);
      }
    }
  }

  $scope.recentlyPlayedSelectNone = function() {
    for(var i = 0; i < $scope.recentlyPlayed.length; i++) {
      if($scope.selectionListIsSelected($scope.recentlyPlayed, i)) {
        $scope.selectionListUnselect($scope.recentlyPlayed, i) 
      }
    }
  }

  $scope.recentlyPlayedSelectAll = function() {
    for(var i = 0; i < $scope.recentlyPlayed.length; i++) {
      if(!$scope.selectionListIsSelected($scope.recentlyPlayed, i)) {
        $scope.selectionListSelect($scope.recentlyPlayed, i);
      }
    }
  }
  /**************** END RECENTLY PLAYED LIST ******************/

  /**************** SELECTION LIST ******************/
  /*
   * Mark the item at `index` in `array` as selected in `selectedMap`. The key used in
   * `selectedMap` is generated by calling the makeKey function with the value from the array.
   *
   */
  $scope.selectionListSelect = function(array, index) {
    if (index >= array.length || index < 0) {
      console.log("$scope.select called with invalid index " + index);
      return;
    }

    array[index].selectionListSelected = true;
  }

  $scope.selectionListUnselect = function(array, index) {
    if (index >= array.length || index < 0) {
      console.log("$scope.unselect called with invalid index " + index);
      return;
    }

    delete array[index]["selectionListSelected"]
  }

  $scope.selectionListIsSelected = function(array, index) {
    if (index >= array.length || index < 0) {
      return false;
    }

    return "selectionListSelected" in (array[index]);
  }

  $scope.selectionListToggle = function(array, index) {

    if ($scope.selectionListIsSelected(array, index)) {
      $scope.selectionListUnselect(array, index);
    } else {
      $scope.selectionListSelect(array, index);
    }
  }

  $scope.selectionListCssClass = function(array, index) {
    if ($scope.selectionListIsSelected(array, index)) {
      return "selected";
    } else {
      return "";
    }
  }

  $scope.selectionListNumSelected = function(array) {
    var c = 0;
    
    for (var i = 0; i < array.length; i++) {
      if ($scope.selectionListIsSelected(array, i)) {
        c++;
      }
    }

    return c;
  }

  /*
   * This function should be called when the array that we are tracking selections in is changed 
   * (elements added, removed, entire list replaced, etc).
   * oldarray should be the array before the change, and newarray the array after. 
   * The parameter nextId is used to assign uniqueIds to the elements in the array which are used
   * to keep track of what's selected and what's not. Any existing uniqueIds from the oldarray are
   * propagated forward to newarray based on the makeKey() function which is used to match up elements
   * from the old and new arrays. Any new elements are given a new unique ID and nextId is incremented.
   *
   */
  $scope.selectionListReplaced = function(oldarray, newarray, makeKey) {
    var oldVals = {};
    for(var i = 0; i < oldarray.length; i++){
      if ("selectionListSelected" in oldarray[i]) {
        oldVals[makeKey(oldarray[i])] = i;
      }
    }

    for(var i = 0; i < newarray.length; i++){
      var k = makeKey(newarray[i]);
      if (k in oldVals) {
        newarray[i].selectionListSelected = true;
      }
    }
  }

  $scope.selectionListSelectedIndexes = function(array) {
    var indexes = [];
    for(var i = 0; i < array.length; i++) {
      if($scope.selectionListIsSelected(array, i)) {
        indexes.push(i);
      }
    }
    return indexes;
  }


  /**************** END SELECTION LIST ******************/


  $scope.playingProp = function(prop){
    if($scope.playing){
      return $scope.playing[prop];
    } else {
      return "" //"-";
    }
  }

  $scope.log = function(s){
    console.log(s);
  }

  $scope.playPauseImg = function(){
    if ($scope.state == "paused"){
      return "play.png"
    } else if ($scope.state == "playing"){
      return "pause.png"
    } else {
      return "play.png"
    }
  }

  $scope.stopClicked = function() {
    playerStopMp3();
  }

  $scope.playPauseClicked = function() {
    if( ! $scope.playing ) {
      $scope.playNext();
    } else {
      if ($scope.state == "playing") {
        playerPauseMp3();
      } else {
        playerPlayMp3();
      }
    }
  }

  $scope.timePosition = function() {
    if( $scope.playing && $scope.playing.sec_per_sample ) {
      return secondsToTime($scope.position*$scope.playing.sec_per_sample)
    } else {
      return "";
    }
  }

  $scope.changeRepeatMode = function() {
    console.log("changeRepeatMode: called");
    if ($scope.repeatMode == "DontRepeat") {
      $scope.sendPlayerSetRepeatModeRequest("RepeatOne")
    } else if ($scope.repeatMode == "RepeatOne") {
      $scope.sendPlayerSetRepeatModeRequest("RepeatAll")
    } else if ($scope.repeatMode == "RepeatAll") {
      $scope.sendPlayerSetRepeatModeRequest("DontRepeat")
    }
  }

  $scope.repeatModeForDisplay = function() {
    if ($scope.repeatMode == "DontRepeat") {
      return "Don't Repeat";
    } else if ($scope.repeatMode == "RepeatOne") {
      return "Repeat One";
    } else if ($scope.repeatMode == "RepeatAll") {
      return "Repeat All";
    }
  }

  $scope.activeIfSomethingSelected = function(selectionList){
    if ($scope.selectionListNumSelected(selectionList) > 0) {
      return "active";
    } else {
      return "inactive";
    }
  }

  $scope.activeIfNothingSelected = function(selectionList){
    if ($scope.selectionListNumSelected(selectionList) > 0) {
      return "inactive";
    } else {
      return "active";
    }
  }

  $scope.activeIfNotAllSelected = function(selectionList){
    if ($scope.selectionListNumSelected(selectionList) != selectionList.length) {
      return "active";
    } else {
      return "inactive";
    }
  }

}
