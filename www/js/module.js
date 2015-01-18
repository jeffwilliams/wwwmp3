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

  $scope.volume = 50;

  // Position in the file
  $scope.position = 50;
  $scope.maxPosition = 100;
  $scope.seekingToPosition = null;
  $scope.seekedAt = null;

  $scope.artists = []
  $scope.albums = []
  $scope.songs = []

  $scope.song = $scope.songs[0];

  // Currently playing mp3
  $scope.playing = null;
  // State of the currently playing mp3
  $scope.state = null;

  $scope.playQueue = [];

  $scope.playerEventsWebsock = null;

  $scope.scannedMp3 = null;

  var fixSelection = function(items, selection, setter){
    if( items.length == 0 ) {
      if ( selection != "" ) {
        setter("");
      }
    } else {
      if( items.indexOf(selection) == -1 ){
        setter(items[0]);
      }
    }
  }

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
        $scope.playNext();
      } else if (state == 1 ){
        $scope.state = "playing";
      } else {
        $scope.state = "paused";
      }
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

  var getMp3Data = function(page, fields, orderField, callback) {
    var parms = {
      'pagesize': 10,
      'page': page,
      'artist' : $scope.artistCriteria,
      'album' : $scope.albumCriteria,
      'title' : $scope.titleCriteria,
      'order' : orderField
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
    getMp3Data($scope.artistPage, ["artist"], "artist", function(data){
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
    getMp3Data($scope.albumPage, ["album"], "album", function(data){
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
    getMp3Data($scope.titlePage, null, "title", function(data){
      $scope.titlePageIsLast = false;
      $scope.songs = []
      for(var i = 0; i < data.length; i++){
        if ( data[i].eof ) {
          $scope.titlePageIsLast = true;
        } else {
          $scope.songs.push( data[i] )
        }
      }

      fixSelection($scope.songs, $scope.song, function(x){console.log("changing song to " + x); $scope.song = x} );
    })
  }

  $scope.setArtistCriteria = function(s) {
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
    var parms = {
      'play': 'play'
    }

    $http.get("/player", {'params' : parms}).
      success(function(data,status,headers,config){
      }).
      error(function(data,status,headers,config){
        console.log("Error: playing mp3 failed: " + data);
      });
  }

  var playerPauseMp3 = function(){
    var parms = {
      'pause': 'pause'
    }

    $http.get("/player", {'params' : parms}).
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

    $http.get("/player", {'params' : parms}).
      success(function(data,status,headers,config){
      }).
      error(function(data,status,headers,config){
        console.log("Error: stopping mp3 failed: " + data);
      });
  }

  var playerGetVolume = function(){
    $http.get("/player", {'params' : {"getvolume":  "getvolume"}}).
      success(function(data,status,headers,config){
        $scope.volume = data.volume
      }).
      error(function(data,status,headers,config){
        console.log("Error: getting volume failed: " + data);
      });
  }

  var volumeThrottler = new Throttler($timeout, 80, 
    function(){
      $http.get("/player", {'params' : {"setvolume": $scope.volume}}).
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
      'seek': $scope.position
    }

    // Mark that we are seeking to a specific position so that 
    // we ignore any intermediate offset change events until we have 
    // received an event with that position. This prevents the button from 
    // jumping around.
    $scope.seekingToPosition = $scope.position;
    $scope.seekedAt = (new Date()).getTime();

    $http.get("/player", {'params' : parms}).
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

  /**************** END PLAYER REQUESTS ******************/
  
  // Initial data load
  getArtists();
  getAlbums();
  getSongs();
  playerGetVolume();

  // Connect websocket
  playerEventsConnect();

  // Whenever one of our filters changes, reload the list of songs to match the filters.
  var filtersChanged = function(newValue, oldValue){
    if(newValue != oldValue){
      $scope.artistPage = 0;
      $scope.albumPage = 0;
      $scope.titlePage = 0;
      
      getArtists();
      getAlbums();
      getSongs();
    }
  }

  $scope.$watch('artistCriteria', filtersChanged);
  $scope.$watch('albumCriteria', filtersChanged);
  $scope.$watch('titleCriteria', filtersChanged);

  $scope.volumeMouseup = function(){
    playerSetVolume();
  }

  $scope.positionMouseup = function(){
    playerSeek();
  }

  /**************** PLAY QUEUE ******************/
  $scope.addSelectedToPlayQueue = function() {
    if($scope.song){
      console.log("$scope.addSelectedToPlayQueue called. song title = " + $scope.song.title);
      $scope.playQueue.push($scope.song);
    }

    $scope.playNext();
  }

  $scope.removeFromPlayQueue = function(s) {
    var i = $scope.playQueue.indexOf(s);
    if( i >= 0 ){
      $scope.playQueue.splice(i,1);
    }
  }

  $scope.moveInPlayQueue = function(index,delta){
    if (delta < 0 ) delta = -1;
    if (delta > 0 ) delta = 1;

    if (index < 1 && delta == -1 || index > $scope.playQueue.length-2 && delta == 1){
      return;
    }
    
    var t = $scope.playQueue[index+delta]
    $scope.playQueue[index+delta] = $scope.playQueue[index];
    $scope.playQueue[index] = t;
  }

  $scope.clearPlayQueue = function(s) {
    $scope.playQueue = [];
  }

  $scope.playNext = function(){
    if(! $scope.playing){
      if($scope.playQueue.length > 0){
        $scope.playing = $scope.playQueue.shift();
        if ( $scope.playing ){
          playerLoadMp3($scope.playing.path, playerPlayMp3);
        }
      }
    }
  }
  /**************** END PLAY QUEUE ******************/


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
    if ($scope.state == "stopped"){
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

}
