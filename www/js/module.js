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

  $scope.artists = []
  $scope.albums = []
  $scope.songs = []

  $scope.song = $scope.songs[0];

  // Currently playing mp3
  $scope.playing = null;
  // State of the currently playing mp3
  $scope.state = null;

  $scope.playQueue = [];

  $scope.playerOffsetWebsock = null;

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

  $scope.userChangingPosition = false;
  var handlePlayerOffsetEvent = function(data){
    if(data == "STOP"){ 
      // Finished mp3.
      $scope.playing = null;
      $scope.state = null;
      $scope.playNext();
    } else {
      $timeout(function(){
        if(! $scope.userChangingPosition ){
          $scope.position = data;
        }
      });
    }
  }

  var playerEventsConnect = function(){
    // Build the websocket URL based on the current window location.
    var loc = window.location, new_uri;
    new_uri = "ws://" + loc.host;
    new_uri += "/playerEvents";

    //socket = new WebSocket("ws://andor:2001/events");
    $scope.playerOffsetWebsock = new WebSocket(new_uri);

    $scope.playerOffsetWebsock.onmessage = function(event){

      // Update the position in the file
      handlePlayerOffsetEvent(event.data);
      //console.log("File offset: " + event.data);
    }

    $scope.playerOffsetWebsock.onopen = function(event){
      console.log("Player offsets: Connected to server")
    }

    $scope.playerOffsetWebsock.onclose = function(event){
      console.log("Player offsets: Connection to server lost")
      window.setTimeout(function(){
        playerEventsConnect();
      }
      , 1000)
    }

    $scope.playerOffsetWebsock.onerror = function(event){
      console.log("Player offsets: socket error: " + event);
    }
  }

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

  var playerLoadMp3 = function(path, callback){
    var parms = {
      'load': path
    }

    $http.get("/player", {'params' : parms}).
      success(function(data,status,headers,config){
        // Close websocket in case we were already connected
        if(null != $scope.playerOffsetWebsock){
          $scope.playerOffsetWebsock.close();
        }
        playerEventsConnect();

        $scope.position = 0; 
        if(data.size){
          $scope.maxPosition = data.size;
        }

        if(callback){
          callback();
        }
      }).
      error(function(data,status,headers,config){
        console.log("Error: loading mp3 failed: " + data);
        $scope.playing = null;
      });
  }

  var playerPlayMp3 = function(){
    var parms = {
      'play': 'play'
    }

    $http.get("/player", {'params' : parms}).
      success(function(data,status,headers,config){
        $scope.state = "playing";
      }).
      error(function(data,status,headers,config){
        console.log("Error: playing mp3 failed: " + data);
        $scope.playing = null;
      });
  }

  var playerPauseMp3 = function(){
    var parms = {
      'pause': 'pause'
    }

    $http.get("/player", {'params' : parms}).
      success(function(data,status,headers,config){
        $scope.state = "paused";
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
        $scope.playing = null;
        $scope.state = null;
      }).
      error(function(data,status,headers,config){
        console.log("Error: stopping mp3 failed: " + data);
      });
  }

  var gettingVolume = false
  var playerGetVolume = function(){
    gettingVolume = true
    $http.get("/player", {'params' : {"getvolume":  "getvolume"}}).
      success(function(data,status,headers,config){
        $scope.volume = data.volume
      }).
      error(function(data,status,headers,config){
        console.log("Error: getting volume failed: " + data);
      });
    gettingVolume = false
  }

  var playerSetVolume = function(){
    $http.get("/player", {'params' : {"setvolume": $scope.volume}}).
      success(function(data,status,headers,config){
      }).
      error(function(data,status,headers,config){
        console.log("Error: setting volume failed: " + data);
      });
  }

  var playerSeek = function(){
    var parms = {
      'seek': $scope.position
    }

    $http.get("/player", {'params' : parms}).
      success(function(data,status,headers,config){
        $scope.userChangingPosition = false;
      }).
      error(function(data,status,headers,config){
        console.log("Error: seeking mp3 failed: " + data);
        $scope.userChangingPosition = false;
      });
  }
  
  // Initial data load
  getArtists();
  getAlbums();
  getSongs();
  playerGetVolume();

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
  $scope.$watch('volume', function(){
    if(! gettingVolume ){
      playerSetVolume();
    }
  });

  $scope.positionMouseup = function(){
    playerSeek();
  }

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

  $scope.playingProp = function(prop){
    if($scope.playing){
      return $scope.playing[prop];
    } else {
      return "-";
    }
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

}
